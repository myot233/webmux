package clicmd

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"strconv"
)

// Minimal RFC 5389 binding exchange. Keeping this stdlib-only preserves nop2p
// dependency pruning. One request, no retransmission, no NAT classification.
func stunBindingRequest() ([]byte, error) {
	request := make([]byte, 20)
	binary.BigEndian.PutUint16(request, 0x0001)
	binary.BigEndian.PutUint32(request[4:], 0x2112a442)
	if _, err := rand.Read(request[8:]); err != nil {
		return nil, fmt.Errorf("STUN transaction: %w", err)
	}
	return request, nil
}

func stunMappedAddress(packet, request []byte) (string, error) {
	if len(packet) < 20 || len(request) != 20 || !bytes.Equal(packet[4:20], request[4:20]) {
		return "", errors.New("invalid STUN cookie or transaction ID")
	}
	size := int(binary.BigEndian.Uint16(packet[2:4]))
	if size%4 != 0 || size != len(packet)-20 {
		return "", errors.New("invalid STUN message length")
	}
	if binary.BigEndian.Uint16(packet[:2]) != 0x0101 {
		return "", errors.New("STUN response is not binding success")
	}
	var mapped string
	for offset := 20; offset < len(packet); {
		if offset+4 > len(packet) {
			return "", errors.New("truncated STUN attribute")
		}
		typ := binary.BigEndian.Uint16(packet[offset:])
		size := int(binary.BigEndian.Uint16(packet[offset+2:]))
		offset += 4
		if offset+((size+3)&^3) > len(packet) {
			return "", errors.New("truncated STUN attribute value")
		}
		value := packet[offset : offset+size]
		offset += (size + 3) &^ 3
		if typ != 0x0020 {
			continue
		} // Require XOR-MAPPED-ADDRESS (RFC 5389).
		if len(value) < 4 || value[0] != 0 {
			return "", errors.New("invalid STUN mapped address")
		}
		n := 0
		switch value[1] {
		case 1:
			n = 4
		case 2:
			n = 16
		}
		if n == 0 || len(value) != n+4 {
			return "", errors.New("invalid STUN address family or length")
		}
		port := binary.BigEndian.Uint16(value[2:]) ^ 0x2112
		ip := make(net.IP, n)
		for i := range ip {
			ip[i] = value[i+4] ^ request[i+4]
		}
		if port == 0 || ip.IsUnspecified() || ip.IsMulticast() {
			return "", errors.New("invalid STUN mapped endpoint")
		}
		mapped = net.JoinHostPort(ip.String(), strconv.Itoa(int(port)))
	}
	if mapped == "" {
		return "", errors.New("STUN success missing XOR-MAPPED-ADDRESS")
	}
	return mapped, nil
}
