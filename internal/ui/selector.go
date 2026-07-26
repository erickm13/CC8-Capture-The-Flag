package ui

import (
	"fmt"
	"image/color"
	"time"

	"ctf/internal/discovery"
	"ctf/internal/protocol"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// ElegirServidorEnVentana abre una ventana con la lista de servidores de la red,
// refrescándose sola, y devuelve la dirección del que el jugador elija con un
// clic. Devuelve "" si cierra la ventana sin elegir.
//
// Es una mini-aplicación Ebitengine independiente de la del juego: se corre
// primero, y cuando el jugador elige, se cierra y el programa principal conecta
// y abre la ventana de juego.
func ElegirServidorEnVentana(dport int, nombre string) string {
	esc := discovery.NuevoEscáner(dport, 1200*time.Millisecond, 2*time.Second)
	esc.Iniciar()
	defer esc.Detener()

	sel := &selector{esc: esc, nombre: nombre}
	ebiten.SetWindowSize(Ancho, Alto)
	ebiten.SetWindowTitle("Captura la Bandera — elegí un servidor")
	// RunGame devuelve cuando el selector pide terminar (con errTerminar).
	if err := ebiten.RunGame(sel); err != nil && err != errTerminar {
		return ""
	}
	return sel.elegido
}

// errTerminar es el error centinela que usa Ebitengine para cerrar el bucle de
// juego de forma controlada cuando el jugador ya eligió.
var errTerminar = fmt.Errorf("selección terminada")

type selector struct {
	esc     *discovery.Escáner
	nombre  string
	elegido string

	filas      []discovery.Servidor // lista dibujada en el último cuadro
	mouseAntes bool
}

const (
	filaAltura = 40
	filaY0     = 120
	filaX      = 40
	filaAncho  = Ancho - 80
)

func (s *selector) Update() error {
	// Detectar clic (flanco de bajada del botón).
	apretado := ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft)
	if apretado && !s.mouseAntes {
		_, my := ebiten.CursorPosition()
		for i := range s.filas {
			y := filaY0 + i*filaAltura
			if my >= y && my < y+filaAltura-6 {
				s.elegido = s.filas[i].Addr()
				return errTerminar // cerramos la ventana de selección
			}
		}
	}
	s.mouseAntes = apretado

	// Salir con Escape sin elegir.
	if ebiten.IsKeyPressed(ebiten.KeyEscape) {
		s.elegido = ""
		return errTerminar
	}
	return nil
}

func (s *selector) Draw(pantalla *ebiten.Image) {
	pantalla.Fill(color.RGBA{18, 18, 24, 255})

	ebitenutil.DebugPrintAt(pantalla, "CAPTURA LA BANDERA", filaX, 30)
	ebitenutil.DebugPrintAt(pantalla, "Elegí un servidor (clic). La lista se actualiza sola. Escape para salir.", filaX, 60)
	ebitenutil.DebugPrintAt(pantalla, fmt.Sprintf("Jugás como: %s", s.nombre), filaX, 84)

	s.filas = s.esc.Lista()
	if len(s.filas) == 0 {
		ebitenutil.DebugPrintAt(pantalla, "Buscando servidores en la red...", filaX, filaY0)
		return
	}

	mx, my := ebiten.CursorPosition()
	for i, sv := range s.filas {
		y := filaY0 + i*filaAltura
		encima := mx >= filaX && mx <= filaX+filaAncho && my >= y && my < y+filaAltura-6

		fondo := color.RGBA{34, 34, 44, 255}
		if encima {
			fondo = color.RGBA{50, 90, 70, 255}
		}
		vector.DrawFilledRect(pantalla, float32(filaX), float32(y),
			float32(filaAncho), float32(filaAltura-6), fondo, true)

		estado := "esperando"
		if sv.State == protocol.StateRunning {
			estado = "jugando"
		}
		linea := fmt.Sprintf("%-20s  %-22s  %s  %d/%d",
			recortarUI(sv.ServerName, 20), sv.Addr(), estado, sv.PlayerCount, sv.MaximumPlayers)
		ebitenutil.DebugPrintAt(pantalla, linea, filaX+10, y+12)
	}
}

func (s *selector) Layout(_, _ int) (int, int) { return Ancho, Alto }

func recortarUI(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
