// Package ui es la interfaz gráfica con Ebitengine (PARTE 6).
//
// Su única responsabilidad es dibujar y leer el teclado. Toda la lógica del
// juego y la red están en internal/client y en el servidor: la interfaz solo
// llama a client.Snapshot() para saber qué dibujar, y a client.Mover() /
// client.Interactuar() cuando el jugador presiona una tecla.
//
// Esa separación es la que hace que la parte visual sea sencilla: no hay estado
// de juego acá, solo pixeles.
package ui

import (
	"fmt"
	"image/color"
	"time"

	"ctf/internal/client"
	"ctf/internal/discovery"
	"ctf/internal/protocol"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// Tamaño de la ventana en píxeles.
const (
	Ancho = 800
	Alto  = 800
)

// modo indica qué pantalla muestra la ventana en cada momento.
type modo int

const (
	modoSeleccion modo = iota // eligiendo servidor de la lista
	modoJuego                 // conectado y jugando
	modoError                 // no se pudo conectar; muestra el mensaje
)

// Juego implementa la interfaz ebiten.Game: Update (lógica por cuadro), Draw
// (dibujo) y Layout (tamaño). Ebitengine llama Update y Draw ~60 veces por
// segundo.
//
// Una sola ventana maneja las dos pantallas (selección y juego) cambiando de
// modo. Es importante NO cerrar y reabrir la ventana: Ebitengine no permite
// llamar a RunGame dos veces en el mismo proceso.
type Juego struct {
	nombre string

	// Estado del juego (modo jugando).
	cli                *client.Cliente
	teclaInteractAntes bool

	// Estado de la selección (modo seleccionando).
	m          modo
	esc        *discovery.Escáner
	dport      int
	filas      []discovery.Servidor
	mouseAntes bool
	errMsg     string
}

// Nuevo crea la interfaz con un cliente ya conectado (arranca jugando). Se usa
// cuando el servidor se eligió por terminal o con -addr.
func Nuevo(cli *client.Cliente, nombre string) *Juego {
	return &Juego{cli: cli, nombre: nombre, m: modoJuego}
}

// NuevoConSeleccion crea la interfaz arrancando en la pantalla de selección de
// servidores. La conexión se hace adentro, al elegir, así la ventana nunca se
// cierra ni se reabre.
func NuevoConSeleccion(nombre string, dport int) *Juego {
	esc := discovery.NuevoEscáner(dport, 1200*time.Millisecond, 2*time.Second)
	esc.Iniciar()
	return &Juego{nombre: nombre, m: modoSeleccion, esc: esc, dport: dport}
}

// Update se llama una vez por cuadro. Aquí leemos el teclado y se lo mandamos al
// servidor a través del cliente. No calculamos posiciones: eso lo hace el
// servidor y nos llega en el Snapshot.
func (g *Juego) Update() error {
	// En modo selección, la ventana muestra la lista y espera un clic.
	if g.m == modoSeleccion {
		return g.updateSeleccion()
	}
	if g.m == modoError {
		if ebiten.IsKeyPressed(ebiten.KeyEscape) {
			return errCerrar
		}
		return nil
	}

	s := g.cli.Snapshot()

	// Si la partida terminó o nos desconectamos, cerrar la ventana.
	if s.Estado == protocol.StateFinished || !s.Conectado {
		// Damos un momento para que se vea el cartel de fin; en un juego real
		// se mostraría una pantalla de resultado. Por simplicidad, seguimos
		// dibujando y no cerramos automáticamente.
	}

	if s.Estado != protocol.StateRunning {
		return nil // en el lobby o la cuenta regresiva no se mueve
	}

	// Leer las flechas (o WASD) y traducirlas a una dirección del protocolo.
	// Si no hay ninguna tecla, la dirección es NONE (quieto).
	dir := uint8(protocol.DirNone)
	switch {
	case ebiten.IsKeyPressed(ebiten.KeyArrowUp) || ebiten.IsKeyPressed(ebiten.KeyW):
		dir = protocol.DirUp
	case ebiten.IsKeyPressed(ebiten.KeyArrowDown) || ebiten.IsKeyPressed(ebiten.KeyS):
		dir = protocol.DirDown
	case ebiten.IsKeyPressed(ebiten.KeyArrowLeft) || ebiten.IsKeyPressed(ebiten.KeyA):
		dir = protocol.DirLeft
	case ebiten.IsKeyPressed(ebiten.KeyArrowRight) || ebiten.IsKeyPressed(ebiten.KeyD):
		dir = protocol.DirRight
	}
	// Mandamos la dirección en cada cuadro; el cliente/servidor la mantienen
	// vigente. Para ahorrar mensajes, un juego más pulido solo la enviaría al
	// cambiar, pero así es más simple y correcto.
	g.cli.Mover(dir)

	// Tecla de interacción: Espacio o Enter. Solo al presionarla (no mientras
	// se mantiene), para no inundar al servidor.
	interact := ebiten.IsKeyPressed(ebiten.KeySpace) || ebiten.IsKeyPressed(ebiten.KeyEnter)
	if interact && !g.teclaInteractAntes {
		g.cli.Interactuar()
	}
	g.teclaInteractAntes = interact

	return nil
}

// Draw dibuja todo el cuadro. Se llama después de Update.
func (g *Juego) Draw(pantalla *ebiten.Image) {
	pantalla.Fill(color.RGBA{18, 18, 24, 255}) // fondo oscuro

	// Despachar según la pantalla actual.
	if g.m == modoSeleccion {
		g.drawSeleccion(pantalla)
		return
	}
	if g.m == modoError {
		ebitenutil.DebugPrintAt(pantalla, "No se pudo conectar al servidor:", 40, 60)
		ebitenutil.DebugPrintAt(pantalla, g.errMsg, 40, 84)
		ebitenutil.DebugPrintAt(pantalla, "Escape para salir.", 40, 120)
		return
	}

	s := g.cli.Snapshot()

	// Antes de que empiece la partida, mostramos el estado del lobby.
	if s.Estado != protocol.StateRunning && s.Estado != protocol.StateFinished {
		g.dibujarLobby(pantalla, s)
		return
	}

	cfg := s.Config
	if cfg.MapSize == 0 {
		return // todavía no llegó GAME_STARTED
	}

	// Conversión de unidades de mundo a píxeles. El mundo va de -MapSize/2 a
	// +MapSize/2; la pantalla de 0 a Ancho. El eje y de la pantalla ya crece
	// hacia abajo, igual que el del protocolo (§5), así que no hay que invertir.
	escala := float64(Ancho) / cfg.MapSize
	aPantalla := func(x, y float64) (float32, float32) {
		px := (x + cfg.MapSize/2) * escala
		py := (y + cfg.MapSize/2) * escala
		return float32(px), float32(py)
	}

	// El círculo central.
	cx, cy := aPantalla(0, 0)
	vector.StrokeCircle(pantalla, cx, cy, float32(cfg.CircleRadius*escala), 2,
		color.RGBA{80, 80, 120, 255}, true)

	// La bandera, si está en el suelo.
	if s.Bandera.Status == protocol.FlagAvailable || s.Bandera.Status == protocol.FlagDropped {
		fx, fy := aPantalla(s.Bandera.X, s.Bandera.Y)
		col := color.RGBA{240, 200, 40, 255} // amarilla
		if s.Bandera.Status == protocol.FlagDropped {
			col = color.RGBA{240, 120, 40, 255} // naranja si está caída
		}
		vector.DrawFilledRect(pantalla, fx-6, fy-10, 12, 20, col, true)
	}

	// Los jugadores.
	for _, p := range s.Jugadores {
		px, py := aPantalla(p.X, p.Y)
		col := colorJugador(p.ID, p.ID == s.MiID)
		radio := float32(cfg.PlayerRadius * escala)
		if radio < 5 {
			radio = 5
		}
		vector.DrawFilledCircle(pantalla, px, py, radio, col, true)

		// Si lleva la bandera, un anillo amarillo alrededor.
		if p.HasFlag {
			vector.StrokeCircle(pantalla, px, py, radio+4, 2, color.RGBA{240, 200, 40, 255}, true)
		}

		// El nombre encima. En GAME_STATE no viene el nombre, pero el propio
		// jugador sabe el suyo; los demás se muestran como P01, P02...
		// El nombre encima de cada jugador. Los nombres se recibieron en
		// GAME_STARTED/LOBBY_STATE; el GAME_STATE no los trae (§29.6). Nombre()
		// los busca por ID y cae en "P01" solo si no lo conoce.
		etiqueta := s.Nombre(p.ID)
		ebitenutil.DebugPrintAt(pantalla, etiqueta, int(px)-10, int(py)-int(radio)-16)
	}

	// Panel de información arriba.
	g.dibujarHUD(pantalla, s)
}

func (g *Juego) dibujarLobby(pantalla *ebiten.Image, s client.Snapshot) {
	y := 40
	titulo := "SALA DE ESPERA"
	if s.Estado == protocol.StateStarting {
		titulo = fmt.Sprintf("¡EMPIEZA EN %d!", s.Countdown)
	}
	ebitenutil.DebugPrintAt(pantalla, titulo, Ancho/2-60, y)
	y += 40
	ebitenutil.DebugPrintAt(pantalla, "Jugadores conectados:", 40, y)
	y += 24
	for _, p := range s.Lobby {
		linea := fmt.Sprintf("  P%02d  %s", p.ID, p.Name)
		if p.ID == s.MiID {
			linea += "  (yo)"
		}
		ebitenutil.DebugPrintAt(pantalla, linea, 40, y)
		y += 20
	}
	ebitenutil.DebugPrintAt(pantalla, "Esperando a que el anfitrión inicie la partida...", 40, Alto-40)
}

func (g *Juego) dibujarHUD(pantalla *ebiten.Image, s client.Snapshot) {
	estado := "bandera libre"
	switch s.Bandera.Status {
	case protocol.FlagCarried:
		estado = fmt.Sprintf("la lleva %s", s.Nombre(s.Bandera.Carrier))
	case protocol.FlagDropped:
		estado = "bandera caída"
	}
	linea := fmt.Sprintf("tick %d   %s   |   flechas o WASD: mover   espacio: tomar/robar",
		s.Tick, estado)
	ebitenutil.DebugPrintAt(pantalla, linea, 8, 8)

	if s.Estado == protocol.StateFinished {
		msg := fmt.Sprintf(">>> GANÓ %s <<<", s.Ganador)
		if s.GanadorID == s.MiID {
			msg = ">>> ¡GANASTE! <<<"
		}
		ebitenutil.DebugPrintAt(pantalla, msg, Ancho/2-60, Alto/2)
	}
}

// colorJugador da un color estable por ID. El jugador propio siempre es verde
// brillante para distinguirlo.
func colorJugador(id uint16, soyYo bool) color.Color {
	if soyYo {
		return color.RGBA{80, 240, 120, 255}
	}
	// Colores derivados del ID para que cada jugador tenga el suyo.
	colores := []color.RGBA{
		{240, 100, 100, 255}, {100, 160, 240, 255}, {240, 160, 60, 255},
		{200, 100, 240, 255}, {100, 220, 220, 255}, {240, 100, 180, 255},
	}
	return colores[int(id)%len(colores)]
}

// Layout fija el tamaño lógico de la pantalla.
func (g *Juego) Layout(anchoExt, altoExt int) (int, int) { return Ancho, Alto }

// errCerrar cierra la ventana de forma controlada (Ebitengine termina RunGame).
var errCerrar = fmt.Errorf("cerrar ventana")

// --------- pantalla de selección de servidores (modo modoSeleccion) ---------

const (
	selFilaAltura = 40
	selFilaY0     = 120
	selFilaX      = 40
	selFilaAncho  = Ancho - 80
)

// updateSeleccion maneja el clic sobre un servidor. Al elegir, conecta y cambia
// a modo juego en la MISMA ventana (no se abre otra).
func (g *Juego) updateSeleccion() error {
	if ebiten.IsKeyPressed(ebiten.KeyEscape) {
		return errCerrar
	}
	apretado := ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft)
	if apretado && !g.mouseAntes {
		_, my := ebiten.CursorPosition()
		for i := range g.filas {
			y := selFilaY0 + i*selFilaAltura
			if my >= y && my < y+selFilaAltura-6 {
				g.conectarA(g.filas[i].Addr())
				break
			}
		}
	}
	g.mouseAntes = apretado
	return nil
}

// conectarA intenta conectarse al servidor elegido y pasa a modo juego. Si falla,
// pasa a modo error. En ningún caso cierra la ventana.
func (g *Juego) conectarA(addr string) {
	cli, err := client.Conectar(addr, g.nombre, client.Eventos{})
	if err != nil {
		g.errMsg = err.Error()
		g.m = modoError
		return
	}
	if g.esc != nil {
		g.esc.Detener() // ya no necesitamos seguir escaneando
		g.esc = nil
	}
	g.cli = cli
	g.m = modoJuego
}

func (g *Juego) drawSeleccion(pantalla *ebiten.Image) {
	ebitenutil.DebugPrintAt(pantalla, "CAPTURA LA BANDERA", selFilaX, 30)
	ebitenutil.DebugPrintAt(pantalla, "Elegí un servidor (clic). La lista se actualiza sola. Escape para salir.", selFilaX, 60)
	ebitenutil.DebugPrintAt(pantalla, fmt.Sprintf("Jugás como: %s", g.nombre), selFilaX, 84)

	if g.esc != nil {
		g.filas = g.esc.Lista()
	}
	if len(g.filas) == 0 {
		ebitenutil.DebugPrintAt(pantalla, "Buscando servidores en la red...", selFilaX, selFilaY0)
		return
	}

	mx, my := ebiten.CursorPosition()
	for i, sv := range g.filas {
		y := selFilaY0 + i*selFilaAltura
		encima := mx >= selFilaX && mx <= selFilaX+selFilaAncho && my >= y && my < y+selFilaAltura-6

		fondo := color.RGBA{34, 34, 44, 255}
		if encima {
			fondo = color.RGBA{50, 90, 70, 255}
		}
		vector.DrawFilledRect(pantalla, float32(selFilaX), float32(y),
			float32(selFilaAncho), float32(selFilaAltura-6), fondo, true)

		estado := "esperando"
		if sv.State == protocol.StateRunning {
			estado = "jugando"
		}
		linea := fmt.Sprintf("%-20s  %-22s  %s  %d/%d",
			recortarUI(sv.ServerName, 20), sv.Addr(), estado, sv.PlayerCount, sv.MaximumPlayers)
		ebitenutil.DebugPrintAt(pantalla, linea, selFilaX+10, y+12)
	}
}

func recortarUI(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
