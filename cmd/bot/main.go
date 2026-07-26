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
	"math/rand"
	"os"
	"strings"
	"time"

	"ctf/internal/client"
	"ctf/internal/discovery"
	"ctf/internal/protocol"
)

// nivel agrupa los parámetros que definen qué tan hábil juega el bot. Se elige
// con la flag -nivel; ver nivelPorNombre para los presets.
type nivel struct {
	nombre       string        // para los logs
	periodo      time.Duration // cada cuánto decide (reacción)
	anticipacion float64       // 0..1: cuánto proyecta el movimiento del portador al perseguir
	reentra      bool          // ¿vuelve a entrar al círculo si robó la bandera afuera? (acuerdo 005)
	histeresis   bool          // ¿suaviza el zig-zag al cambiar de eje?
	ruido        float64       // error de puntería en el movimiento (0 = perfecto)
	distraccion  float64       // 0..1: probabilidad por vuelta de despistarse (moverse al azar o quedarse quieto)
}

// nivelPorNombre traduce el valor de -nivel a sus parámetros. Acepta variantes
// comunes (con y sin acento, en inglés).
func nivelPorNombre(s string) (nivel, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "facil", "fácil", "easy":
		return nivel{"facil", 400 * time.Millisecond, 0.0, false, false, 110, 0.4}, nil
	case "intermedio", "medio", "medium":
		return nivel{"intermedio", 150 * time.Millisecond, 0.5, true, true, 0, 0.0}, nil
	case "avanzado", "dificil", "difícil", "hard":
		return nivel{"avanzado", 100 * time.Millisecond, 1.0, true, true, 0, 0.0}, nil
	}
	return nivel{}, fmt.Errorf("nivel desconocido %q (usá facil|intermedio|avanzado)", s)
}

func main() {
	addr := flag.String("addr", "", "dirección del servidor; vacío = descubrir")
	dport := flag.Int("discovery-port", 5001, "puerto UDP del descubrimiento")
	name := flag.String("name", "bot", "nombre del jugador")
	nivelFlag := flag.String("nivel", "avanzado", "dificultad de juego: facil|intermedio|avanzado")
	debug := flag.Bool("debug", false, "mostrar cada mensaje en hex con desglose byte por byte")
	save := flag.Bool("save", false, "guardar el log de cada partida en un archivo (carpeta logs/)")
	flag.Parse()

	n, err := nivelPorNombre(*nivelFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	protocol.DebugActivo = *debug
	if *save {
		protocol.IniciarGuardado("cliente")
		defer protocol.CerrarGuardado()
	}

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
	logf(*name, "conectado (nivel %s), esperando el inicio...", n.nombre)

	jugar(c, *name, n)
	logf(*name, "listo")
}

// jugar es el bucle de decisión. El nivel define el ritmo y la habilidad.
func jugar(c *client.Cliente, nombre string, n nivel) {
	t := time.NewTicker(n.periodo)
	defer t.Stop()
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	var ultimaDir uint8 = 255
	teniaBandera := false   // ¿llevaba la bandera en la vuelta anterior?
	entroDesdeToma := false // ¿entré al círculo desde que la tomé? (acuerdo 005)

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

			// Rastrear el requisito de victoria: hay que haber estado dentro del
			// círculo *desde* que se tomó la bandera. El servidor reinicia esa
			// marca en cada toma/robo (giveFlag), así que la reproducimos: al
			// detectar el flanco de subida de HasFlag, arrancamos según dónde
			// estemos, y luego la confirmamos cada vez que entramos al círculo.
			if yo.HasFlag {
				dentro := math.Hypot(yo.X, yo.Y) <= s.Config.CircleRadius
				if !teniaBandera {
					entroDesdeToma = dentro
				} else if dentro {
					entroDesdeToma = true
				}
			}
			teniaBandera = yo.HasFlag

			objX, objY, interactuar := decidir(s, *yo, entroDesdeToma, n.anticipacion, n.reentra)

			// Presionar la tecla si estoy a tiro del objetivo real (sin ruido).
			d := math.Hypot(objX-yo.X, objY-yo.Y)
			if interactuar && d <= s.Config.InteractionRadius {
				c.Interactuar()
			}

			// En niveles bajos la puntería es imperfecta: desviamos el objetivo
			// del movimiento (no el del interactuar) con un error aleatorio.
			movX, movY := objX, objY
			if n.ruido > 0 {
				movX += (rng.Float64()*2 - 1) * n.ruido
				movY += (rng.Float64()*2 - 1) * n.ruido
			}

			// Elegir la dirección de 4 que más me acerca al objetivo. Como no
			// hay diagonales, voy por el eje donde estoy más lejos. Sin
			// histéresis (niveles bajos) el bot zig-zaguea más.
			previa := ultimaDir
			if !n.histeresis {
				previa = 255
			}
			dir := direccionHacia(yo.X, yo.Y, movX, movY, previa)

			// Distracción (nivel fácil): a veces se despista y va para cualquier
			// lado o se queda quieto en vez de perseguir el objetivo.
			if n.distraccion > 0 && rng.Float64() < n.distraccion {
				if rng.Intn(2) == 0 {
					continue // se queda quieto esta vuelta
				}
				dir = protocol.DirUp + uint8(rng.Intn(4)) // una de las 4 al azar
			}

			if dir != ultimaDir {
				c.Mover(dir)
				ultimaDir = dir
			}
		}
	}
}

// decidir elige el objetivo: si llevo la bandera, el borde; si la lleva otro,
// hacia él; si está libre, hacia ella.
//
// entroDesdeToma indica si ya cumplí el requisito de victoria de haber estado
// dentro del círculo desde que tomé la bandera. Si todavía no (típico al robarla
// afuera), primero vuelvo a entrar; recién después salgo a ganar.
//
// anticipacion (0..1) escala cuánto proyecto el movimiento del portador al
// perseguir. reentra activa la vuelta al círculo del acuerdo 005; si está en
// falso el bot corre siempre afuera (juego ingenuo de nivel fácil).
func decidir(s client.Snapshot, yo protocol.Player, entroDesdeToma bool, anticipacion float64, reentra bool) (x, y float64, interactuar bool) {
	cfg := s.Config
	if yo.HasFlag {
		d := math.Hypot(yo.X, yo.Y)
		if reentra && !entroDesdeToma {
			// Todavía no entré al círculo con la bandera: apunto al punto de la
			// frontera más cercano hacia adentro (más rápido que ir al centro).
			dentro := cfg.CircleRadius - cfg.PlayerRadius - 20
			if d < 1 || dentro <= 0 {
				return 0, 0, false // ya estoy en el centro, dentro del círculo
			}
			return yo.X / d * dentro, yo.Y / d * dentro, false
		}
		// Ya cumplí el requisito: salgo por el punto más cercano fuera del
		// círculo, en la dirección en la que ya estoy respecto del centro.
		fuera := cfg.CircleRadius + cfg.PlayerRadius + 40
		if d < 1 {
			return fuera, 0, false // estoy en el centro; salgo hacia la derecha
		}
		return yo.X / d * fuera, yo.Y / d * fuera, false
	}
	if p := s.Portador(); p != nil {
		// Se la robo, pero apunto a dónde *estará*, no a dónde está: como se
		// mueve, perseguir su posición actual siempre lo deja por detrás.
		px, py := interceptar(yo, *p, cfg.PlayerSpeed, anticipacion)
		return px, py, true
	}
	return s.Bandera.X, s.Bandera.Y, true // la tomo
}

// interceptar estima dónde alcanzar al portador. Ambos van a la misma
// velocidad, así que uso el tiempo de vuelo hacia su posición actual como
// anticipación y proyecto su movimiento en su dirección de rumbo. anticipacion
// escala esa proyección (0 = perseguir la posición actual, sin anticipar).
func interceptar(yo, obj protocol.Player, velocidad, anticipacion float64) (x, y float64) {
	if velocidad <= 0 || anticipacion <= 0 {
		return obj.X, obj.Y
	}
	vx, vy := velocidadDe(obj.Direction, velocidad)
	t := math.Hypot(obj.X-yo.X, obj.Y-yo.Y) / velocidad // segundos de vuelo
	return obj.X + vx*t*anticipacion, obj.Y + vy*t*anticipacion
}

// velocidadDe traduce una dirección de 4 (§) a un vector de velocidad.
func velocidadDe(dir uint8, velocidad float64) (vx, vy float64) {
	switch dir {
	case protocol.DirUp:
		return 0, -velocidad
	case protocol.DirDown:
		return 0, velocidad
	case protocol.DirLeft:
		return -velocidad, 0
	case protocol.DirRight:
		return velocidad, 0
	}
	return 0, 0
}

// direccionHacia elige una de las 4 direcciones. Va por el eje donde la
// distancia al objetivo es mayor, para acercarse en línea recta por tramos.
//
// ultima es la dirección actual: aplica histéresis para no alternar de eje ante
// diferencias mínimas (el zig-zag que aparece cuando dx ≈ dy). Solo cambia de
// eje si el otro es claramente mayor, o si ya quedé alineado en el eje actual.
func direccionHacia(x, y, objX, objY float64, ultima uint8) uint8 {
	dx, dy := objX-x, objY-y
	adx, ady := math.Abs(dx), math.Abs(dy)
	const margen = 20.0 // banda muerta de la histéresis

	horizontal := adx >= ady
	switch ultima {
	case protocol.DirLeft, protocol.DirRight:
		horizontal = adx+margen >= ady && adx > 1
	case protocol.DirUp, protocol.DirDown:
		horizontal = adx > ady+margen || ady <= 1
	}
	if horizontal {
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
