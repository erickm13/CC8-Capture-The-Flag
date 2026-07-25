// discover: busca servidores de Captura la Bandera en la red (PARTE 4).
//
// Manda un DISCOVER_REQUEST por broadcast y lista los servidores que responden.
// Es lo que le permite a un cliente conectarse sin escribir la IP a mano, uno
// de los entregables del reglamento.
//
//	go run ./cmd/discover
//
// Para probarlo: levantá un servidor con descubrimiento en otra terminal
//
//	go run ./cmd/server -autostart 3 -small
//
// y después corré discover: debería aparecer esa partida en la lista.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"ctf/internal/discovery"
	"ctf/internal/protocol"
)

func main() {
	port := flag.Int("discovery-port", 5001, "puerto UDP del descubrimiento")
	esperar := flag.Duration("wait", 1500*time.Millisecond, "cuánto esperar respuestas")
	flag.Parse()

	fmt.Printf("buscando servidores en UDP %d durante %s...\n\n", *port, *esperar)

	servidores, err := discovery.Buscar(*port, *esperar)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	if len(servidores) == 0 {
		fmt.Println("No se encontró ningún servidor.")
		fmt.Println("(¿está corriendo un servidor con descubrimiento en esta red?)")
		return
	}

	fmt.Printf("%-24s %-22s %-10s %s\n", "SERVIDOR", "DIRECCIÓN", "ESTADO", "JUGADORES")
	fmt.Println("--------------------------------------------------------------------------")
	for _, s := range servidores {
		fmt.Printf("%-24s %-22s %-10s %d/%d\n",
			recortar(s.ServerName, 24), s.Addr(), estado(s.State), s.PlayerCount, s.MaximumPlayers)
	}
	fmt.Printf("\n%d servidor(es) encontrado(s). Conectate con:\n", len(servidores))
	fmt.Printf("  go run ./cmd/sonda -addr %s\n", servidores[0].Addr())
}

func estado(s uint8) string {
	switch s {
	case protocol.StateWaiting:
		return "esperando"
	case protocol.StateRunning:
		return "jugando"
	default:
		return "?"
	}
}

func recortar(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
