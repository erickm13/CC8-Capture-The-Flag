// Servidor de Captura la Bandera (PARTE 3).
//
// Lo ejecuta el anfitrión. Acepta conexiones TCP, arma el lobby, corre la
// cuenta regresiva y la partida, y difunde el estado a todos los clientes.
//
// Ejemplos:
//
//	go run ./cmd/server                 # arranca con Enter, mundo normal
//	go run ./cmd/server -autostart 2    # arranca solo al llegar 2 jugadores
//	go run ./cmd/server -small          # mundo chico (rápido de probar)
//
// Para conectarte necesitás un cliente. En esta parte, la sonda de cmd/sonda
// sirve como cliente de prueba:
//
//	go run ./cmd/sonda -addr 127.0.0.1:5000
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"ctf/internal/game"
	"ctf/internal/server"
)

func main() {
	port := flag.Int("port", 5000, "puerto TCP")
	dport := flag.Int("discovery-port", 5001, "puerto UDP del descubrimiento")
	name := flag.String("name", "Partida de prueba", "nombre del servidor")
	autostart := flag.Int("autostart", 0, "arrancar al llegar a esta cantidad de jugadores (0 = con Enter)")
	small := flag.Bool("small", false, "usar el mundo chico y rápido de las demos")
	flag.Parse()

	cfg := game.DefaultConfig()
	if *small {
		cfg.MapSize, cfg.CircleRadius = 800, 200
		cfg.PlayerSpeed, cfg.TickIntervalMs = 400, 100
	}

	s := server.New(server.Options{
		ServerName:    *name,
		TCPPort:       *port,
		Config:        cfg,
		CountdownSecs: 3,
		AutoStart:     *autostart,
	})
	if err := s.Listen(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	// El descubrimiento es deseable pero no imprescindible: si el puerto está
	// ocupado (por ejemplo, otro servidor en la misma máquina), seguimos igual.
	if err := s.ListenDiscovery(*dport); err != nil {
		fmt.Fprintln(os.Stderr, "aviso:", err, "— se podrá conectar por IP manual")
	}

	// Con -autostart la partida arranca sola; sin él, el anfitrión presiona Enter.
	if *autostart == 0 {
		go func() {
			fmt.Println(">>> Presioná Enter para iniciar la partida <<<")
			fmt.Scanln()
			s.Start()
		}()
	}

	// Cerrar limpio con Ctrl-C.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() { <-sig; fmt.Println("\ncerrando..."); s.Close(); os.Exit(0) }()

	s.Run()
}
