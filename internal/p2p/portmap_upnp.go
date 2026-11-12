package p2p

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	ssdpMulticast  = "239.255.255.250:1900"
	upnpMaxXMLSize = 128 * 1024
)

func (m *portMapper) mapUPnP(ctx context.Context) (*mappingLease, error) {
	m.mu.Lock()
	descURL := m.upnpOverride
	m.mu.Unlock()

	if descURL == "" {
		var err error
		descURL, err = discoverUPnPLocation(ctx)
		if err != nil {
			return nil, err
		}
	}
	ctrlURL, svcType, localIP, err := resolveUPnPControlURL(ctx, descURL)
	if err != nil {
		return nil, err
	}
	if err := upnpSOAPAddPortMapping(ctx, ctrlURL, svcType, m.internalUDP, localIP); err != nil {
		return nil, err
	}
	extIP := upnpSOAPGetExternalIP(ctx, ctrlURL, svcType)
	port := m.internalUDP
	return &mappingLease{
		protocol:     "upnp",
		externalIP:   extIP,
		externalPort: port,
		release: func(relCtx context.Context) {
			_ = upnpSOAPDeletePortMapping(relCtx, ctrlURL, svcType, port)
		},
	}, nil
}

func discoverUPnPLocation(ctx context.Context) (string, error) {
	conn, err := net.ListenPacket("udp4", ":0")
	if err != nil {
		return "", err
	}
	defer func() { _ = conn.Close() }()
	dst, err := net.ResolveUDPAddr("udp4", ssdpMulticast)
	if err != nil {
		return "", err
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(900 * time.Millisecond)
	}
	_ = conn.SetDeadline(deadline)
	for _, st := range []string{
		"urn:schemas-upnp-org:service:WANIPConnection:2",
		"urn:schemas-upnp-org:service:WANIPConnection:1",
		"urn:schemas-upnp-org:service:WANPPPConnection:1",
	} {
		msg := "M-SEARCH * HTTP/1.1\r\nHOST: " + ssdpMulticast + "\r\nMAN: \"ssdp:discover\"\r\nMX: 1\r\nST: " + st + "\r\n\r\n"
		_, _ = conn.WriteTo([]byte(msg), dst)
	}
	buf := make([]byte, 2048)
	for {
		n, _, err := conn.ReadFrom(buf)
		if err != nil {
			return "", err
		}
		for _, line := range strings.Split(string(buf[:n]), "\r\n") {
			if len(line) > 9 && strings.EqualFold(line[:9], "location:") {
				loc := strings.TrimSpace(line[9:])
				if strings.HasPrefix(strings.ToLower(loc), "http://") {
					return loc, nil
				}
			}
		}
	}
}

type upnpRootXML struct {
	Services []struct {
		ServiceType string `xml:"serviceType"`
		ControlURL  string `xml:"controlURL"`
	} `xml:"device>deviceList>device>deviceList>device>serviceList>service"`
	DirectServices []struct {
		ServiceType string `xml:"serviceType"`
		ControlURL  string `xml:"controlURL"`
	} `xml:"device>serviceList>service"`
}

func resolveUPnPControlURL(ctx context.Context, descURL string) (string, string, string, error) {
	u, err := url.Parse(descURL)
	if err != nil || u.Scheme != "http" {
		return "", "", "", errors.New("invalid upnp location")
	}
	hostIP := net.ParseIP(u.Hostname())
	if hostIP != nil && !hostIP.IsPrivate() && !hostIP.IsLoopback() {
		return "", "", "", errors.New("upnp location is not on local network")
	}
	localIP := ""
	if d, err := net.Dial("udp4", net.JoinHostPort(u.Hostname(), "80")); err == nil {
		if la, ok := d.LocalAddr().(*net.UDPAddr); ok {
			localIP = la.IP.String()
		}
		_ = d.Close()
	}
	if localIP == "" {
		return "", "", "", errors.New("cannot determine local IP for upnp")
	}

	client := &http.Client{Timeout: time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, descURL, nil)
	if err != nil {
		return "", "", "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", "", err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, upnpMaxXMLSize))
	if err != nil {
		return "", "", "", err
	}
	var doc upnpRootXML
	if err := xml.Unmarshal(raw, &doc); err != nil {
		return "", "", "", err
	}
	candidates := append(doc.Services, doc.DirectServices...)
	for _, s := range candidates {
		if strings.Contains(s.ServiceType, "WANIPConnection:") || strings.Contains(s.ServiceType, "WANPPPConnection:") {
			rel, err := url.Parse(strings.TrimSpace(s.ControlURL))
			if err != nil {
				continue
			}
			return u.ResolveReference(rel).String(), strings.TrimSpace(s.ServiceType), localIP, nil
		}
	}
	return "", "", "", errors.New("no WANIPConnection service in upnp description")
}

func upnpSOAPCall(ctx context.Context, ctrlURL, svcType, action, innerXML string) ([]byte, error) {
	body := `<?xml version="1.0"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"><s:Body><u:` +
		action + ` xmlns:u="` + svcType + `">` + innerXML + `</u:` + action + `></s:Body></s:Envelope>`
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ctrlURL, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", `text/xml; charset="utf-8"`)
	req.Header.Set("SOAPAction", `"`+svcType+`#`+action+`"`)
	client := &http.Client{Timeout: time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	out, _ := io.ReadAll(io.LimitReader(resp.Body, upnpMaxXMLSize))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("upnp %s status %d", action, resp.StatusCode)
	}
	return out, nil
}

func upnpSOAPAddPortMapping(ctx context.Context, ctrlURL, svcType string, port int, localIP string) error {
	p := strconv.Itoa(port)
	inner := `<NewRemoteHost></NewRemoteHost><NewExternalPort>` + p + `</NewExternalPort><NewProtocol>UDP</NewProtocol><NewInternalPort>` +
		p + `</NewInternalPort><NewInternalClient>` + localIP + `</NewInternalClient><NewEnabled>1</NewEnabled><NewPortMappingDescription>WebMux P2P</NewPortMappingDescription><NewLeaseDuration>` +
		strconv.Itoa(leaseSeconds) + `</NewLeaseDuration>`
	_, err := upnpSOAPCall(ctx, ctrlURL, svcType, "AddPortMapping", inner)
	return err
}

func upnpSOAPDeletePortMapping(ctx context.Context, ctrlURL, svcType string, port int) error {
	p := strconv.Itoa(port)
	inner := `<NewRemoteHost></NewRemoteHost><NewExternalPort>` + p + `</NewExternalPort><NewProtocol>UDP</NewProtocol>`
	_, err := upnpSOAPCall(ctx, ctrlURL, svcType, "DeletePortMapping", inner)
	return err
}

func upnpSOAPGetExternalIP(ctx context.Context, ctrlURL, svcType string) net.IP {
	raw, err := upnpSOAPCall(ctx, ctrlURL, svcType, "GetExternalIPAddress", "")
	if err != nil {
		return nil
	}
	type extIPResp struct {
		IP string `xml:"Body>GetExternalIPAddressResponse>NewExternalIPAddress"`
	}
	var parsed extIPResp
	if xml.Unmarshal(raw, &parsed) == nil {
		return net.ParseIP(strings.TrimSpace(parsed.IP))
	}
	return nil
}
