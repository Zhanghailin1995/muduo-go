package muduo

import (
	"golang.org/x/sys/unix"
	"muduo/pkg/errors"
	"muduo/pkg/logging"
	"net"
	"syscall"
)

type socket struct {
	fd int
}

func newSocket() *socket {
	fd, err := sysSocket(unix.AF_INET, unix.SOCK_STREAM, unix.IPPROTO_TCP)
	if err != nil {
		panic(err)
	}
	return &socket{fd: fd}
}

func (s *socket) bind(network, addr string) (unix.Sockaddr, error) {
	sa, _, _, _, err := GetTCPSockAddr(network, addr)
	if err != nil {
		return nil, err
	}
	err = unix.Bind(s.fd, sa)
	if err != nil {
		panic(err)
	}
	return sa, nil
}

func (s *socket) accept() (int, net.Addr, error) {
	fd, sa, err := unix.Accept4(s.fd, syscall.SOCK_NONBLOCK|syscall.SOCK_CLOEXEC)
	if err != nil {
		switch err {
		case unix.EINTR, unix.EAGAIN, unix.ECONNABORTED:
			// ECONNABORTED means that a socket on the listen
			// queue was closed before we Accept()ed it;
			// it's a silly error, so try again.
			return -1, nil, err
		default:
			logging.Errorf("Accept() failed due to error: %v", err)
			return -1, nil, errors.ErrAcceptSocket
		}
	}
	remoteAddr := SockaddrToTCPOrUnixAddr(sa)
	return fd, remoteAddr, nil
}

func (s *socket) close() error {
	return unix.Close(s.fd)
}

func (s *socket) listen() {
	err := unix.Listen(s.fd, unix.SOMAXCONN)
	if err != nil {
		panic(err)
	}
}

func (s *socket) setReuseAddr(f bool) error {
	flag := 1
	if !f {
		flag = 0
	}
	return unix.SetsockoptInt(s.fd, unix.SOL_SOCKET, unix.SO_REUSEADDR, flag)
}

func sysSocket(family, sotype, proto int) (int, error) {
	return unix.Socket(family, sotype|unix.SOCK_NONBLOCK|unix.SOCK_CLOEXEC, proto)
}

// SockaddrToTCPOrUnixAddr converts a Sockaddr to a net.TCPAddr or net.UnixAddr.
// Returns nil if conversion fails.
func SockaddrToTCPOrUnixAddr(sa unix.Sockaddr) net.Addr {
	switch sa := sa.(type) {
	case *unix.SockaddrInet4:
		return &net.TCPAddr{IP: sa.Addr[0:], Port: sa.Port}
	case *unix.SockaddrInet6:
		return &net.TCPAddr{IP: sa.Addr[0:], Port: sa.Port, Zone: ip6ZoneToString(sa.ZoneId)}
	case *unix.SockaddrUnix:
		return &net.UnixAddr{Name: sa.Name, Net: "unix"}
	}
	return nil
}

// ip6ZoneToString converts an IP6 Zone unix int to a net string,
// returns "" if zone is 0.
func ip6ZoneToString(zone uint32) string {
	if zone == 0 {
		return ""
	}
	if ifi, err := net.InterfaceByIndex(int(zone)); err == nil {
		return ifi.Name
	}
	return uint2decimalStr(uint(zone))
}

// uint2decimalStr converts val to a decimal string.
func uint2decimalStr(val uint) string {
	if val == 0 { // avoid string allocation
		return "0"
	}
	buf := make([]byte, 20) // big enough for 64bit value base 10
	i := len(buf) - 1
	for val >= 10 {
		q := val / 10
		buf[i] = byte('0' + val - q*10)
		i--
		val = q
	}
	// val < 10
	buf[i] = byte('0' + val)
	return string(buf[i:])
}

// GetTCPSockAddr the structured addresses based on the protocol and raw address.
func GetTCPSockAddr(proto, addr string) (sa unix.Sockaddr, family int, tcpAddr *net.TCPAddr, ipv6only bool, err error) {
	var tcpVersion string

	tcpAddr, err = net.ResolveTCPAddr(proto, addr)
	if err != nil {
		return
	}

	tcpVersion, err = determineTCPProto(proto, tcpAddr)
	if err != nil {
		return
	}

	switch tcpVersion {
	case "tcp4":
		family = unix.AF_INET
		sa, err = ipToSockaddr(family, tcpAddr.IP, tcpAddr.Port, "")
	case "tcp6":
		ipv6only = true
		fallthrough
	case "tcp":
		family = unix.AF_INET6
		sa, err = ipToSockaddr(family, tcpAddr.IP, tcpAddr.Port, tcpAddr.Zone)
	default:
		err = errors.ErrUnsupportedProtocol
	}

	return
}

func determineTCPProto(proto string, addr *net.TCPAddr) (string, error) {
	// If the protocol is set to "tcp", we try to determine the actual protocol
	// version from the size of the resolved IP address. Otherwise, we simple use
	// the protocol given to us by the caller.

	if addr.IP.To4() != nil {
		return "tcp4", nil
	}

	if addr.IP.To16() != nil {
		return "tcp6", nil
	}

	switch proto {
	case "tcp", "tcp4", "tcp6":
		return proto, nil
	}

	return "", errors.ErrUnsupportedTCPProtocol
}

func ipToSockaddr(family int, ip net.IP, port int, zone string) (unix.Sockaddr, error) {
	switch family {
	case syscall.AF_INET:
		sa, err := ipToSockaddrInet4(ip, port)
		if err != nil {
			return nil, err
		}
		return &sa, nil
	case syscall.AF_INET6:
		sa, err := ipToSockaddrInet6(ip, port, zone)
		if err != nil {
			return nil, err
		}
		return &sa, nil
	}
	return nil, &net.AddrError{Err: "invalid address family", Addr: ip.String()}
}

func ipToSockaddrInet4(ip net.IP, port int) (unix.SockaddrInet4, error) {
	if len(ip) == 0 {
		ip = net.IPv4zero
	}
	ip4 := ip.To4()
	if ip4 == nil {
		return unix.SockaddrInet4{}, &net.AddrError{Err: "non-IPv4 address", Addr: ip.String()}
	}
	sa := unix.SockaddrInet4{Port: port}
	copy(sa.Addr[:], ip4)
	return sa, nil
}

func ipToSockaddrInet6(ip net.IP, port int, zone string) (unix.SockaddrInet6, error) {
	// In general, an IP wildcard address, which is either
	// "0.0.0.0" or "::", means the entire IP addressing
	// space. For some historical reason, it is used to
	// specify "any available address" on some operations
	// of IP node.
	//
	// When the IP node supports IPv4-mapped IPv6 address,
	// we allow a listener to listen to the wildcard
	// address of both IP addressing spaces by specifying
	// IPv6 wildcard address.
	if len(ip) == 0 || ip.Equal(net.IPv4zero) {
		ip = net.IPv6zero
	}
	// We accept any IPv6 address including IPv4-mapped
	// IPv6 address.
	ip6 := ip.To16()
	if ip6 == nil {
		return unix.SockaddrInet6{}, &net.AddrError{Err: "non-IPv6 address", Addr: ip.String()}
	}

	sa := unix.SockaddrInet6{Port: port}
	copy(sa.Addr[:], ip6)
	iface, err := net.InterfaceByName(zone)
	if err != nil {
		return sa, nil
	}
	sa.ZoneId = uint32(iface.Index)

	return sa, nil
}
