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
	"flag"
	"fmt"
	"os"
	"time"

	"ctf/internal/client"
	"ctf/internal/discovery"
	"ctf/internal/ui"

	"github.com/hajimehoshi/ebiten/v2"
)

func main() {
	addr := flag.String("addr", "", "dirección del servidor; vacío = descubrir")
	dport := flag.Int("discovery-port", 5001, "puerto UDP del descubrimiento")
	name := flag.String("name", "jugador", "tu nombre")
	flag.Parse()

	// Buscar el servidor si no se dio dirección.
	destino := *addr
	if destino == "" {
		fmt.Println("buscando servidores en la red...")
		servidores, err := discovery.Buscar(*dport, 1500*time.Millisecond)
		if err != nil || len(servidores) == 0 {
			fmt.Fprintln(os.Stderr, "no se encontró servidor; usá -addr host:puerto")
			os.Exit(1)
		}
		destino = servidores[0].Addr()
		fmt.Printf("servidor encontrado: %q en %s\n", servidores[0].ServerName, destino)
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
