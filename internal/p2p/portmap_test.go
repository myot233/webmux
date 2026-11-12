package p2p

import (
	"context"
	"encoding/binary"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestPCPMappingAndRelease(t *testing.T) {
	srv, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	var releases atomic.Int32
	go func() {
		buf := make([]byte, 128)
		for {
			n, remote, err := srv.ReadFromUDP(buf)
			if err != nil {
				return
			}
			if n < 60 || buf[0] != 2 || buf[1] != 1 {
				continue
			}
			lifetime := binary.BigEndian.Uint32(buf[4:8])
			if lifetime == 0 {
				releases.Add(1)
			}
			resp := make([]byte, 60)
			resp[0] = 2
			resp[1] = 0x81 // response bit | MAP
			resp[3] = 0    // success
			binary.BigEndian.PutUint32(resp[4:8], lifetime)
			copy(resp[24:36], buf[24:36]) // nonce
			resp[36] = 17
			copy(resp[40:42], buf[40:42])
			binary.BigEndian.PutUint16(resp[42:44], 45678)
			resp[54], resp[55] = 0xff, 0xff
			copy(resp[56:60], net.IPv4(203, 0, 113, 9).To4())
			_, _ = srv.WriteToUDP(resp, remote)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	target := srv.LocalAddr().(*net.UDPAddr)
	lease, err := mapPCPWithLifetime(ctx, target, 45678, 600)
	if err != nil {
		t.Fatal(err)
	}
	if lease.protocol != "pcp" || lease.externalPort != 45678 || lease.externalIP.String() != "203.0.113.9" {
		t.Fatalf("unexpected pcp lease: %+v", lease)
	}
	lease.release(ctx)
	if releases.Load() != 1 {
		t.Fatalf("expected 1 PCP release packet, got %d", releases.Load())
	}
}

func TestNATPMPMappingAndRelease(t *testing.T) {
	srv, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	var releases atomic.Int32
	go func() {
		buf := make([]byte, 64)
		for {
			n, remote, err := srv.ReadFromUDP(buf)
			if err != nil {
				return
			}
			if n == 2 && buf[0] == 0 && buf[1] == 0 {
				resp := make([]byte, 12)
				resp[0], resp[1] = 0, 128
				copy(resp[8:12], net.IPv4(198, 51, 100, 42).To4())
				_, _ = srv.WriteToUDP(resp, remote)
				continue
			}
			if n == 12 && buf[0] == 0 && buf[1] == 1 {
				lifetime := binary.BigEndian.Uint32(buf[8:12])
				if lifetime == 0 {
					releases.Add(1)
				}
				resp := make([]byte, 16)
				resp[0], resp[1] = 0, 129
				copy(resp[8:10], buf[4:6])
				binary.BigEndian.PutUint16(resp[10:12], 51234)
				binary.BigEndian.PutUint32(resp[12:16], lifetime)
				_, _ = srv.WriteToUDP(resp, remote)
			}
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	target := srv.LocalAddr().(*net.UDPAddr)
	lease, err := mapNATPMPAddr(ctx, target, 51234, 600)
	if err != nil {
		t.Fatal(err)
	}
	if lease.protocol != "nat-pmp" || lease.externalPort != 51234 || lease.externalIP.String() != "198.51.100.42" {
		t.Fatalf("unexpected nat-pmp lease: %+v", lease)
	}
	lease.release(ctx)
	if releases.Load() != 1 {
		t.Fatalf("expected 1 NAT-PMP release packet, got %d", releases.Load())
	}
}

func TestUPnPIGDMappingAndFallback(t *testing.T) {
	var added, deleted atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/rootDesc.xml", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?>
<root xmlns="urn:schemas-upnp-org:device-1-0">
  <device>
    <serviceList>
      <service>
        <serviceType>urn:schemas-upnp-org:service:WANIPConnection:1</serviceType>
        <controlURL>/ctl/IPConn</controlURL>
      </service>
    </serviceList>
  </device>
</root>`))
	})
	mux.HandleFunc("/ctl/IPConn", func(w http.ResponseWriter, r *http.Request) {
		action := r.Header.Get("SOAPAction")
		switch {
		case strings.Contains(action, "AddPortMapping"):
			added.Add(1)
			w.WriteHeader(200)
		case strings.Contains(action, "GetExternalIPAddress"):
			w.WriteHeader(200)
			_, _ = w.Write([]byte(`<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body><u:GetExternalIPAddressResponse xmlns:u="urn:schemas-upnp-org:service:WANIPConnection:1"><NewExternalIPAddress>203.0.113.77</NewExternalIPAddress></u:GetExternalIPAddressResponse></s:Body></s:Envelope>`))
		case strings.Contains(action, "DeletePortMapping"):
			deleted.Add(1)
			w.WriteHeader(200)
		default:
			w.WriteHeader(400)
		}
	})
	igd := httptest.NewServer(mux)
	defer igd.Close()

	doneCh := make(chan PortMapStatus, 4)
	pm := newPortMapper(49152, func(_ string, st PortMapStatus) {
		if st.State == "mapped" || st.State == "unavailable" {
			select {
			case doneCh <- st:
			default:
			}
		}
	})
	pm.gatewayOverride = net.IPv4(127, 0, 0, 1) // no PCP/NAT-PMP on 127.0.0.1:5351 -> falls back to UPnP
	pm.upnpOverride = igd.URL + "/rootDesc.xml"
	pm.SetEnabled(true)

	select {
	case st := <-doneCh:
		if st.State != "mapped" || st.Protocol != "upnp" || st.ExternalPort != 49152 {
			t.Fatalf("expected mapped upnp status, got %+v", st)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for UPnP port map")
	}

	_, extIP := pm.Status()
	if extIP != "203.0.113.77" {
		t.Fatalf("expected mapped external IP 203.0.113.77, got %q", extIP)
	}
	pm.Close()
	if added.Load() != 1 || deleted.Load() != 1 {
		t.Fatalf("expected add=1 delete=1, got add=%d delete=%d", added.Load(), deleted.Load())
	}
}
