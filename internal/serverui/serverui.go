// Package serverui es la interfaz gráfica del ANFITRIÓN.
//
// El servidor no juega (§4): no tiene entidad en el mapa. Pero sí coordina y
// muestra, y hasta ahora "mostrar" era leer líneas de log en la terminal. Esta
// ventana hace las dos cosas que le faltaban:
//
//   - iniciar la partida con un botón, en vez de escribir 'start';
//   - ver la partida en vivo, con el mismo mapa que ven los jugadores.
//
// No agrega ningún jugador ni toca la lógica: llama a server.Vista() una vez por
// cuadro para saber qué dibujar, y a server.Start() cuando se aprieta el botón.
//
// La ventana es la consola del anfitrión, así que cerrarla (con la X o con
// Escape) apaga el servidor, igual que un Ctrl-C en la terminal.
package serverui

import (
	"fmt"
	"image/color"
	"unicode/utf8"

	"ctf/internal/protocol"
	"ctf/internal/server"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// Medidas de la ventana. A la izquierda el mapa (cuadrado, como el mundo); a la
// derecha el panel con los datos del servidor, los jugadores y el botón.
const (
	Ancho = 1120
	Alto  = 760

	mapaX    = 20
	mapaY    = 20
	mapaLado = 700

	panelX     = mapaX + mapaLado + 20 // 740
	panelAncho = Ancho - panelX - 20   // 360

	btnAlto = 46
	btnY    = Alto - 20 - btnAlto

	logY      = 430 // arranque del registro de eventos
	altoLinea = 16
)

// Paleta. Se define acá y no se comparte con internal/ui a propósito: esta
// pantalla es del anfitrión y conviene que se distinga de la del jugador.
var (
	colFondo    = color.RGBA{16, 16, 22, 255}
	colPanel    = color.RGBA{26, 26, 36, 255}
	colBorde    = color.RGBA{70, 70, 95, 255}
	colCirculo  = color.RGBA{80, 80, 120, 255}
	colBandera  = color.RGBA{240, 200, 40, 255}
	colCaida    = color.RGBA{240, 120, 40, 255}
	colBotonOn  = color.RGBA{40, 120, 70, 255}
	colBotonMou = color.RGBA{55, 160, 95, 255}
	colBotonOff = color.RGBA{45, 45, 58, 255}
)

// Observador implementa ebiten.Game. Solo tiene la referencia al servidor y el
// estado mínimo para detectar el flanco de las teclas y el mouse: todo lo que
// dibuja sale de Vista().
type Observador struct {
	srv *server.Server

	mouseAntes bool
	teclaAntes bool
}

// Nuevo crea la ventana de observación para un servidor ya escuchando.
func Nuevo(s *server.Server) *Observador { return &Observador{srv: s} }

// Layout fija el tamaño lógico de la pantalla.
func (o *Observador) Layout(anchoExt, altoExt int) (int, int) { return Ancho, Alto }

// ErrCerrar es lo que devuelve Update cuando el anfitrión cierra la ventana con
// Escape. Quien llama a ebiten.RunGame lo trata como un cierre normal, no como
// un error.
var ErrCerrar = fmt.Errorf("cerrar ventana del servidor")

// Update lee el mouse y el teclado. Lo único que puede hacer el anfitrión desde
// acá es iniciar la partida; el resto es mirar.
func (o *Observador) Update() error {
	// Detectar el flanco: nos interesa el momento en que se aprieta, no todos
	// los cuadros que se mantiene apretado. Se calcula siempre, incluso sin
	// foco, para que el clic que trae la ventana al frente no cuente como un
	// flanco nuevo apenas la recupera.
	apretado := ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft)
	tecla := ebiten.IsKeyPressed(ebiten.KeyEnter) || ebiten.IsKeyPressed(ebiten.KeySpace)
	flancoMouse := apretado && !o.mouseAntes
	flancoTecla := tecla && !o.teclaAntes
	o.mouseAntes, o.teclaAntes = apretado, tecla

	// Ebitengine sigue llamando a Update aunque la ventana esté atrás, y el
	// mouse se lee igual. Sin este corte, un clic en OTRA aplicación que caiga
	// sobre las coordenadas del botón inicia la partida sin que el anfitrión
	// toque esta ventana (pasa de verdad: se probó). Sin foco, solo se mira.
	if !ebiten.IsFocused() {
		return nil
	}

	if ebiten.IsKeyPressed(ebiten.KeyEscape) {
		return ErrCerrar
	}
	if !o.srv.PuedeIniciar() {
		return nil
	}
	// Enter y espacio hacen lo mismo que el botón, para no soltar el teclado.
	if flancoTecla || (flancoMouse && enBoton(ebiten.CursorPosition())) {
		o.srv.Start()
	}
	return nil
}

// enBoton dice si el cursor está sobre el botón de inicio.
func enBoton(mx, my int) bool {
	return mx >= panelX && mx <= panelX+panelAncho && my >= btnY && my <= btnY+btnAlto
}

// Draw dibuja el cuadro completo: mapa a la izquierda, panel a la derecha.
func (o *Observador) Draw(pantalla *ebiten.Image) {
	pantalla.Fill(colFondo)
	v := o.srv.Vista()
	o.dibujarMapa(pantalla, v)
	o.dibujarPanel(pantalla, v)
}

// ---------- el mapa (la partida en vivo) ----------

func (o *Observador) dibujarMapa(pantalla *ebiten.Image, v server.Vista) {
	vector.StrokeRect(pantalla, mapaX, mapaY, mapaLado, mapaLado, 1, colBorde, true)

	// Antes de que arranque la partida no hay nada que mirar: los jugadores se
	// colocan en el mapa recién al empezar (SpawnAll), así que durante el lobby
	// y la cuenta regresiva sus posiciones son las de la partida anterior, o el
	// centro en la primera. Dibujarlas sería mentir; mejor el mapa vacío.
	if v.Estado == protocol.StateWaiting || v.Estado == protocol.StateStarting || v.Cfg.MapSize == 0 {
		o.mapaVacio(pantalla, v)
		return
	}

	// Conversión de unidades de mundo a píxeles del recuadro del mapa. El mundo
	// va de -MapSize/2 a +MapSize/2; el eje y de la pantalla ya crece hacia
	// abajo igual que el del protocolo (§5), así que no hay que invertirlo.
	escala := float64(mapaLado) / v.Cfg.MapSize
	aPantalla := func(x, y float64) (float32, float32) {
		return float32(mapaX + (x+v.Cfg.MapSize/2)*escala),
			float32(mapaY + (y+v.Cfg.MapSize/2)*escala)
	}

	// El círculo central: hay que salir de ahí con la bandera para ganar (§16).
	cx, cy := aPantalla(0, 0)
	vector.StrokeCircle(pantalla, cx, cy, float32(v.Cfg.CircleRadius*escala), 2, colCirculo, true)

	// La bandera, si está en el suelo (si la llevan, se ve en el portador).
	if v.Bandera.Status == protocol.FlagAvailable || v.Bandera.Status == protocol.FlagDropped {
		fx, fy := aPantalla(v.Bandera.X, v.Bandera.Y)
		col := colBandera
		if v.Bandera.Status == protocol.FlagDropped {
			col = colCaida
		}
		vector.DrawFilledRect(pantalla, fx-6, fy-10, 12, 20, col, true)
	}

	// Los jugadores, con su nombre encima y un anillo si llevan la bandera.
	radio := float32(v.Cfg.PlayerRadius * escala)
	if radio < 5 {
		radio = 5
	}
	for _, p := range v.Jugadores {
		px, py := aPantalla(p.X, p.Y)
		vector.DrawFilledCircle(pantalla, px, py, radio, colorJugador(p.ID), true)
		if p.LlevaBandera {
			vector.StrokeCircle(pantalla, px, py, radio+4, 2, colBandera, true)
		}
		etiqueta := fmt.Sprintf("%s (P%02d)", p.Nombre, p.ID)
		ebitenutil.DebugPrintAt(pantalla, etiqueta,
			int(px)-anchoTexto(etiqueta)/2, int(py)-int(radio)-18)
	}

	// Al terminar, quién ganó, encima del mapa congelado en la jugada final.
	if v.Estado == protocol.StateFinished {
		cartel(pantalla, fmt.Sprintf(">>> GANÓ %s <<<", v.Ganador))
	}
}

// mapaVacio es lo que se ve mientras no hay partida: el círculo y un mensaje,
// sin jugadores. Cubre el lobby y la cuenta regresiva.
func (o *Observador) mapaVacio(pantalla *ebiten.Image, v server.Vista) {
	if v.Cfg.MapSize > 0 {
		escala := float64(mapaLado) / v.Cfg.MapSize
		vector.StrokeCircle(pantalla, mapaX+mapaLado/2, mapaY+mapaLado/2,
			float32(v.Cfg.CircleRadius*escala), 2, colCirculo, true)
	}

	titulo, msg := "SALA DE ESPERA", "la partida se ve acá en cuanto arranque"
	switch {
	case v.Estado == protocol.StateStarting:
		titulo = fmt.Sprintf("EMPIEZA EN %d", v.Countdown)
		msg = "colocando a los jugadores en el mapa..."
	case len(v.Jugadores) == 0:
		msg = "esperando que se conecte alguien..."
	case v.AutoStart > 0:
		msg = fmt.Sprintf("arranca sola al llegar a %d jugador(es)", v.AutoStart)
	}

	cartel(pantalla, titulo)
	ebitenutil.DebugPrintAt(pantalla, msg,
		mapaX+(mapaLado-anchoTexto(msg))/2, mapaY+mapaLado/2+24)
}

// cartel escribe un texto grande y centrado sobre el mapa, con una banda oscura
// detrás para que se lea aunque haya jugadores debajo.
func cartel(pantalla *ebiten.Image, texto string) {
	y := mapaY + mapaLado/2 - 10
	vector.DrawFilledRect(pantalla, mapaX+1, float32(y-8), mapaLado-2, 30,
		color.RGBA{16, 16, 22, 240}, true)
	ebitenutil.DebugPrintAt(pantalla, texto, mapaX+(mapaLado-anchoTexto(texto))/2, y)
}

// ---------- el panel de la derecha ----------

func (o *Observador) dibujarPanel(pantalla *ebiten.Image, v server.Vista) {
	vector.DrawFilledRect(pantalla, panelX, mapaY, panelAncho, Alto-2*mapaY, colPanel, true)

	x := panelX + 14
	y := mapaY + 14

	linea := func(formato string, a ...any) {
		ebitenutil.DebugPrintAt(pantalla, fmt.Sprintf(formato, a...), x, y)
		y += altoLinea
	}

	linea("SERVIDOR")
	linea("%s", recortar(v.Nombre, 40))
	y += 6
	linea("escuchando en  %s", v.Addr)
	linea("gameId %d   estado: %s", v.GameID, nombreEstado(v.Estado, v.Countdown))
	linea("jugadores %d/%d   tick %d", len(v.Jugadores), v.MaxPlayers, v.Tick)
	y += 12

	linea("JUGADORES CONECTADOS")
	if len(v.Jugadores) == 0 {
		linea("  (nadie todavía)")
	}
	// El panel tiene lugar acotado: si hay mucha gente, se corta y se avisa
	// cuántos quedaron afuera en vez de escribir encima del registro.
	const maxFilas = 12
	for i, p := range v.Jugadores {
		if i == maxFilas {
			linea("  ... y %d más", len(v.Jugadores)-maxFilas)
			break
		}
		marca := " "
		if p.LlevaBandera {
			marca = "*" // lleva la bandera
		}
		linea("%s P%02d  %s", marca, p.ID, recortar(p.Nombre, 24))
	}

	// El registro de eventos va anclado a una altura fija: así no se mueve
	// cuando entra o sale un jugador de la lista de arriba.
	y = logY
	linea("ÚLTIMOS EVENTOS")
	for _, e := range v.Eventos {
		ebitenutil.DebugPrintAt(pantalla, "  "+recortar(e, 54), x, y)
		y += altoLinea
	}

	o.dibujarBoton(pantalla, v)
}

// dibujarBoton pinta el botón de inicio. Solo está activo en el lobby y con al
// menos un jugador: iniciar una partida vacía no tiene sentido.
func (o *Observador) dibujarBoton(pantalla *ebiten.Image, v server.Vista) {
	activo := v.Estado == protocol.StateWaiting && len(v.Jugadores) > 0

	col := colBotonOff
	texto := "INICIAR PARTIDA"
	switch {
	case activo && enBoton(ebiten.CursorPosition()):
		col = colBotonMou
	case activo:
		col = colBotonOn
	case v.Estado == protocol.StateWaiting:
		texto = "ESPERANDO JUGADORES"
	case v.Estado == protocol.StateStarting:
		texto = fmt.Sprintf("EMPIEZA EN %d...", v.Countdown)
	case v.Estado == protocol.StateRunning:
		texto = "PARTIDA EN CURSO"
	case v.Estado == protocol.StateFinished:
		texto = "MOSTRANDO RESULTADO"
	}

	vector.DrawFilledRect(pantalla, panelX, btnY, panelAncho, btnAlto, col, true)
	vector.StrokeRect(pantalla, panelX, btnY, panelAncho, btnAlto, 1, colBorde, true)
	ebitenutil.DebugPrintAt(pantalla, texto,
		panelX+(panelAncho-anchoTexto(texto))/2, btnY+btnAlto/2-8)

	// La ayuda solo menciona el inicio cuando de verdad se puede iniciar; si no,
	// invitaría a apretar un botón que no hace nada.
	ayuda := "Escape cierra esta ventana"
	if activo {
		ayuda = "clic o Enter para iniciar   ·   Escape cierra esta ventana"
	}
	ebitenutil.DebugPrintAt(pantalla, ayuda,
		panelX+(panelAncho-anchoTexto(ayuda))/2, btnY-20)
}

// ---------- ayudantes ----------

func nombreEstado(e uint8, countdown uint8) string {
	switch e {
	case protocol.StateWaiting:
		return "en el lobby"
	case protocol.StateStarting:
		return fmt.Sprintf("arrancando (%d)", countdown)
	case protocol.StateRunning:
		return "jugando"
	case protocol.StateFinished:
		return "terminada"
	case protocol.StateCancelled:
		return "cancelada"
	}
	return "?"
}

// colorJugador da un color estable por ID, el mismo criterio que usa la
// interfaz del jugador para que el anfitrión reconozca a cada uno.
func colorJugador(id uint16) color.Color {
	colores := []color.RGBA{
		{240, 100, 100, 255}, {100, 160, 240, 255}, {240, 160, 60, 255},
		{200, 100, 240, 255}, {100, 220, 220, 255}, {240, 100, 180, 255},
	}
	return colores[int(id)%len(colores)]
}

// anchoTexto estima el ancho en píxeles del texto con la fuente de depuración de
// Ebitengine, que es de ancho fijo (6 px por carácter). Sirve para centrar.
func anchoTexto(s string) int { return utf8.RuneCountInString(s) * 6 }

// recortar acorta a n caracteres (no bytes: los nombres pueden tener acentos).
func recortar(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n-1]) + "…"
}
