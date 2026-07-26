// juego: el cliente con interfaz gráfica (PARTE 6).
//
// Conecta a un servidor (por descubrimiento o -addr) y abre una ventana donde
// se juega con el teclado. Es el modo cliente del §4.
//
// Ejemplos:
//
//	go run ./cmd/juego -name Ana                 # busca servidor por broadcast
//	go run ./cmd/juego -name Beto -addr 127.0.0.1:5000
//
// Controles: flechas o WASD para moverse, espacio para tomar/robar la bandera.
//
// NOTA: este programa necesita Ebitengine, que usa las librerías gráficas del
// sistema (en Arch: libx11, libgl, etc., normalmente ya instaladas si tenés
// entorno de escritorio). La primera vez, Go las descargará con:
//
//	go mod tidy
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"ctf/internal/client"
	"ctf/internal/discovery"
	"ctf/internal/protocol"
	"ctf/internal/ui"

	"github.com/hajimehoshi/ebiten/v2"
)

func main() {
	addr := flag.String("addr", "", "dirección del servidor; vacío = elegir de una lista")
	dport := flag.Int("discovery-port", 5001, "puerto UDP del descubrimiento")
	name := flag.String("name", "jugador", "tu nombre")
	menu := flag.String("menu", "terminal", "cómo elegir servidor sin -addr: 'terminal' o 'ventana'")
	flag.Parse()

	destino := *addr

	// Si no se dio -addr, elegir un servidor. Dos vías:
	//   -menu terminal (por defecto): lista numerada en la consola
	//   -menu ventana: pantalla de selección dentro de la ventana del juego
	if destino == "" && *menu == "ventana" {
		// La selección gráfica la maneja la interfaz: le pasamos un escáner y
		// la ventana muestra la lista hasta que el jugador elija. Conectamos
		// después, ya adentro de la interfaz.
		elegido := ui.ElegirServidorEnVentana(*dport, *name)
		if elegido == "" {
			fmt.Println("no se eligió ningún servidor.")
			return
		}
		destino = elegido
	} else if destino == "" {
		destino = elegirServidorEnTerminal(*dport)
		if destino == "" {
			fmt.Println("no se eligió ningún servidor.")
			return
		}
	}

	// Conectar. Los eventos se imprimen en la terminal además de dibujarse.
	cli, err := client.Conectar(destino, *name, client.Eventos{
		AlTomar:    func(t uint32, id uint16) { fmt.Printf("P%02d tomó la bandera (tick %d)\n", id, t) },
		AlRobar:    func(t uint32, p, n uint16) { fmt.Printf("P%02d le robó a P%02d\n", n, p) },
		AlTerminar: func(gid uint16, nom string) { fmt.Printf("ganó %s\n", nom) },
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "error al conectar:", err)
		os.Exit(1)
	}
	defer cli.Cerrar()
	fmt.Printf("conectado como %s. Abriendo ventana...\n", *name)

	// Abrir la ventana de juego.
	ebiten.SetWindowSize(ui.Ancho, ui.Alto)
	ebiten.SetWindowTitle("Captura la Bandera — " + *name)
	juego := ui.Nuevo(cli, *name)
	if err := ebiten.RunGame(juego); err != nil {
		fmt.Fprintln(os.Stderr, "error de la interfaz:", err)
		os.Exit(1)
	}
}

// elegirServidorEnTerminal muestra una lista de servidores que se refresca sola
// y deja al jugador elegir por número. Devuelve la dirección elegida, o "" si el
// jugador cancela con 'q'. Escribiendo 'r' se fuerza un refresco visual.
func elegirServidorEnTerminal(dport int) string {
	esc := discovery.NuevoEscáner(dport, 1200*time.Millisecond, 2*time.Second)
	esc.Iniciar()
	defer esc.Detener()

	fmt.Println("Buscando servidores en la red... (la lista se actualiza sola)")
	fmt.Println("Escribí el número para conectarte, 'r' para refrescar, 'q' para salir.")
	fmt.Println()

	// Un hilo redibuja la lista cada 2 s para que el jugador vea aparecer y
	// desaparecer servidores mientras decide.
	dibujar := func() []discovery.Servidor {
		lista := esc.Lista()
		fmt.Print("\n=== Servidores disponibles ===\n")
		if len(lista) == 0 {
			fmt.Println("  (ninguno todavía... esperá unos segundos)")
		}
		for i, s := range lista {
			estado := "esperando"
			if s.State == protocol.StateRunning {
				estado = "jugando"
			}
			fmt.Printf("  [%d] %-20s %-22s %s  %d/%d\n",
				i+1, recortar(s.ServerName, 20), s.Addr(), estado, s.PlayerCount, s.MaximumPlayers)
		}
		fmt.Print("\nElegí un número (o r/q): ")
		return lista
	}

	// Redibujo automático en segundo plano.
	pararDibujo := make(chan struct{})
	go func() {
		t := time.NewTicker(2 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-pararDibujo:
				return
			case <-t.C:
				dibujar()
			}
		}
	}()
	defer close(pararDibujo)

	dibujar()
	sc := bufio.NewScanner(os.Stdin)
	for sc.Scan() {
		txt := strings.TrimSpace(strings.ToLower(sc.Text()))
		switch txt {
		case "q", "quit", "salir":
			return ""
		case "r", "":
			dibujar()
			continue
		}
		n, err := strconv.Atoi(txt)
		lista := esc.Lista()
		if err != nil || n < 1 || n > len(lista) {
			fmt.Printf("número inválido. Elegí entre 1 y %d (o r/q): ", len(lista))
			continue
		}
		elegido := lista[n-1]
		fmt.Printf("conectando a %q en %s...\n", elegido.ServerName, elegido.Addr())
		return elegido.Addr()
	}
	return ""
}

func recortar(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
