// bot: un jugador automático que juega de verdad (PARTE 5).
//
// A diferencia de la sonda, este calcula hacia dónde moverse a partir de su
// posición real y la de la bandera. Sirve para probar el servidor con jugadores
// que compiten, y como ejemplo de cómo usar el paquete client.
//
// Ejemplos:
//
//	go run ./cmd/bot -name Ana                 # encuentra el servidor por broadcast
//	go run ./cmd/bot -name Beto -addr 127.0.0.1:5000
//
// Estrategia: si no tengo la bandera, voy hacia ella (o hacia quien la lleva) y
// presiono la tecla al estar cerca. Si la tengo, corro al punto más cercano
// fuera del círculo para ganar.
package main

import (
	"flag"
	"fmt"
	"math"
	"os"
	"time"

	"ctf/internal/client"
	"ctf/internal/discovery"
	"ctf/internal/protocol"
)

func main() {
	addr := flag.String("addr", "", "dirección del servidor; vacío = descubrir")
	dport := flag.Int("discovery-port", 5001, "puerto UDP del descubrimiento")
	name := flag.String("name", "bot", "nombre del jugador")
	flag.Parse()

	destino := *addr
	if destino == "" {
		servidores, err := discovery.Buscar(*dport, 1500*time.Millisecond)
		if err != nil || len(servidores) == 0 {
			fmt.Fprintln(os.Stderr, "no se encontró servidor; usá -addr host:puerto")
			os.Exit(1)
		}
		destino = servidores[0].Addr()
		logf(*name, "servidor encontrado en %s", destino)
	}

	c, err := client.Conectar(destino, *name, client.Eventos{
		AlTomar: func(t uint32, id uint16) { logf(*name, "P%02d tomó la bandera (tick %d)", id, t) },
		AlRobar: func(t uint32, p, n uint16) { logf(*name, "P%02d le robó a P%02d (tick %d)", n, p, t) },
		AlTerminar: func(gid uint16, nom string) {
			logf(*name, "fin de la partida, ganó %s", nom)
		},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	defer c.Cerrar()
	logf(*name, "conectado, esperando el inicio...")

	jugar(c, *name)
	logf(*name, "listo")
}

// jugar es el bucle de decisión, a 100 ms por vuelta.
func jugar(c *client.Cliente, nombre string) {
	t := time.NewTicker(100 * time.Millisecond)
	defer t.Stop()
	var ultimaDir uint8 = 255

	for {
		select {
		case <-c.Listo():
			return
		case <-t.C:
			s := c.Snapshot()
			if s.Estado == protocol.StateFinished {
				return
			}
			if s.Estado != protocol.StateRunning {
				continue
			}
			yo := s.Yo()
			if yo == nil {
				continue
			}

			objX, objY, interactuar := decidir(s, *yo)

			// Presionar la tecla si estoy a tiro del objetivo.
			d := math.Hypot(objX-yo.X, objY-yo.Y)
			if interactuar && d <= s.Config.InteractionRadius {
				c.Interactuar()
			}

			// Elegir la dirección de 4 que más me acerca al objetivo. Como no
			// hay diagonales, voy por el eje donde estoy más lejos.
			dir := direccionHacia(yo.X, yo.Y, objX, objY)
			if dir != ultimaDir {
				c.Mover(dir)
				ultimaDir = dir
			}
		}
	}
}

// decidir elige el objetivo: si llevo la bandera, el borde; si la lleva otro,
// hacia él; si está libre, hacia ella.
func decidir(s client.Snapshot, yo protocol.Player) (x, y float64, interactuar bool) {
	cfg := s.Config
	if yo.HasFlag {
		// Ir al punto más cercano fuera del círculo, en la dirección en la que
		// ya estoy respecto del centro.
		d := math.Hypot(yo.X, yo.Y)
		fuera := cfg.CircleRadius + cfg.PlayerRadius + 40
		if d < 1 {
			return fuera, 0, false // estoy en el centro; salgo hacia la derecha
		}
		return yo.X / d * fuera, yo.Y / d * fuera, false
	}
	if p := s.Portador(); p != nil {
		return p.X, p.Y, true // se la robo
	}
	return s.Bandera.X, s.Bandera.Y, true // la tomo
}

// direccionHacia elige una de las 4 direcciones. Va por el eje donde la
// distancia al objetivo es mayor, para acercarse en línea recta por tramos.
func direccionHacia(x, y, objX, objY float64) uint8 {
	dx, dy := objX-x, objY-y
	if math.Abs(dx) >= math.Abs(dy) {
		if dx >= 0 {
			return protocol.DirRight
		}
		return protocol.DirLeft
	}
	if dy >= 0 {
		return protocol.DirDown
	}
	return protocol.DirUp
}

func logf(nombre, format string, a ...any) {
	fmt.Printf("%s [%s] %s\n", time.Now().Format("15:04:05.000"), nombre,
		fmt.Sprintf(format, a...))
}
