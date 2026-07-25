// Package discovery implementa la búsqueda de servidores por UDP broadcast
// desde el lado del cliente (§19).
package discovery

import (
	"fmt"
	"net"
	"time"

	"ctf/internal/protocol"
)

// Servidor encontrado por el descubrimiento.
type Servidor struct {
	protocol.DiscoverResponse
	IP net.IP
}

// Addr arma la dirección TCP a la que conectarse. La IP sale del datagrama
// recibido (§27.2), no del mensaje.
func (s Servidor) Addr() string {
	return net.JoinHostPort(s.IP.String(), fmt.Sprint(s.TCPPort))
}

// Buscar manda un DISCOVER_REQUEST por broadcast y junta las respuestas durante
// el tiempo indicado. Devuelve un servidor por gameId (evita duplicados cuando
// un servidor responde por varias interfaces).
func Buscar(port int, esperar time.Duration) ([]Servidor, error) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{Port: 0})
	if err != nil {
		return nil, fmt.Errorf("no se pudo abrir el socket UDP: %w", err)
	}
	defer conn.Close()

	req, _ := protocol.MarshalBinary(protocol.DiscoverRequest{})

	// Se manda a la dirección de broadcast global y también a loopback, porque
	// en Linux el broadcast no siempre le llega a un servidor de la misma
	// máquina, que es el caso al probar solo.
	destinos := []*net.UDPAddr{
		{IP: net.IPv4bcast, Port: port},
		{IP: net.IPv4(127, 0, 0, 1), Port: port},
	}
	for _, d := range destinos {
		conn.WriteToUDP(req, d) // si un destino falla, el otro puede andar
	}

	encontrados := map[uint16]Servidor{}
	conn.SetReadDeadline(time.Now().Add(esperar))
	buf := make([]byte, 2048)
	for {
		n, from, err := conn.ReadFromUDP(buf)
		if err != nil {
			break // venció el plazo
		}
		msg, err := protocol.UnmarshalBinary(buf[:n])
		if err != nil {
			continue
		}
		resp, ok := msg.(protocol.DiscoverResponse)
		if !ok {
			continue
		}
		encontrados[resp.GameID] = Servidor{DiscoverResponse: resp, IP: from.IP}
	}

	out := make([]Servidor, 0, len(encontrados))
	for _, s := range encontrados {
		out = append(out, s)
	}
	return out, nil
}
