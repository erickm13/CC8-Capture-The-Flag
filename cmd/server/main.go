// Servidor de Captura la Bandera (PARTE 3).
//
// Lo ejecuta el anfitrión. Acepta conexiones TCP, arma el lobby, corre la
// cuenta regresiva y la partida, y difunde el estado a todos los clientes.
//
// Ejemplos:
//
//	go run ./cmd/server                 # arranca con Enter, mundo normal
//	go run ./cmd/server -ui             # con ventana: botón de inicio y partida en vivo
//	go run ./cmd/server -autostart 2    # arranca solo al llegar 2 jugadores
//	go run ./cmd/server -small          # mundo chico (rápido de probar)
//
// Para conectarte necesitás un cliente. En esta parte, la sonda de cmd/sonda
// sirve como cliente de prueba:
//
//	go run ./cmd/sonda -addr 127.0.0.1:5000
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"ctf/internal/game"
	"ctf/internal/protocol"
	"ctf/internal/server"
	"ctf/internal/serverui"

	"github.com/hajimehoshi/ebiten/v2"
)

func main() {
	port := flag.Int("port", 5000, "puerto TCP")
	dport := flag.Int("discovery-port", 5001, "puerto UDP del descubrimiento")
	name := flag.String("name", "Partida de prueba", "nombre del servidor")
	autostart := flag.Int("autostart", 0, "opcional: arrancar solo al llegar a N jugadores (0 = manual con 'start')")
	maxPlayers := flag.Int("max", 100, "máximo de jugadores admitidos")
	postgame := flag.Int("postgame", 5, "segundos que se muestra el resultado antes de volver al lobby")
	small := flag.Bool("small", false, "usar el mundo chico y rápido de las demos")
	conUI := flag.Bool("ui", false, "abrir la ventana del anfitrión: botón de inicio y partida en vivo")
	debug := flag.Bool("debug", false, "mostrar cada mensaje en hex con desglose byte por byte")
	save := flag.Bool("save", false, "guardar el log de cada partida en un archivo (carpeta logs/)")
	flag.Parse()

	protocol.DebugActivo = *debug
	if *save {
		protocol.IniciarGuardado("servidor")
		defer protocol.CerrarGuardado()
	}

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
		PostGameSecs:  *postgame,
		AutoStart:     *autostart,
		MaxPlayers:    *maxPlayers,
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

	// Sin -autostart, el anfitrión inicia cada partida escribiendo 'start' en
	// esta terminal. El lector sigue activo entre partidas, y también cuando hay
	// ventana: los dos caminos terminan llamando a s.Start().
	if *autostart == 0 {
		go func() {
			fmt.Println(">>> Escribí 'start' y Enter para iniciar la partida (con los jugadores que haya) <<<")
			sc := bufio.NewScanner(os.Stdin)
			for sc.Scan() {
				linea := strings.TrimSpace(strings.ToLower(sc.Text()))
				switch linea {
				case "start", "s", "":
					fmt.Println(">>> recibí 'start', iniciando...")
					s.Start()
				case "salir", "quit", "exit":
					protocol.CerrarGuardado()
					s.Close()
					os.Exit(0)
				default:
					fmt.Printf(">>> no entendí %q. Escribí 'start' para iniciar.\n", linea)
				}
			}
			// Si el scanner termina (stdin cerrado), avisamos: sin esto, la
			// terminal quedaría muda y parecería que 'start' no funciona.
			if err := sc.Err(); err != nil {
				fmt.Fprintln(os.Stderr, "lector de comandos terminó:", err)
			}
		}()
	}

	// Cerrar limpio con Ctrl-C.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() { <-sig; fmt.Println("\ncerrando..."); protocol.CerrarGuardado(); s.Close(); os.Exit(0) }()

	if *conUI {
		correrConVentana(s, *name)
		return
	}
	s.Run()
}

// correrConVentana levanta la interfaz del anfitrión. El bucle de partidas pasa
// a una goroutine porque Ebitengine exige quedarse con la goroutine principal.
// Cerrar la ventana apaga el servidor: es la consola del anfitrión.
func correrConVentana(s *server.Server, nombre string) {
	go s.Run()

	ebiten.SetWindowSize(serverui.Ancho, serverui.Alto)
	ebiten.SetWindowTitle("Servidor — " + nombre)
	err := ebiten.RunGame(serverui.Nuevo(s))

	fmt.Println("cerrando el servidor...")
	protocol.CerrarGuardado()
	s.Close()
	if err != nil && err != serverui.ErrCerrar {
		fmt.Fprintln(os.Stderr, "error de la interfaz:", err)
		os.Exit(1)
	}
}
