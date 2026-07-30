# Documentación del uso de IA — Captura la Bandera (CC8)

Este documento registra el uso de inteligencia artificial en el desarrollo del
proyecto, como pide el reglamento del curso. Recoge los *prompts* (las
indicaciones que le di al asistente), un resumen de sus respuestas, y los
fragmentos de código más importantes que surgieron de ese trabajo.

**Herramienta usada:** Claude (Anthropic), a través de su interfaz de chat con
ejecución de código.

**Cómo se trabajó:** el asistente no solo escribió código; en cada paso lo
compiló y lo ejecutó en su propio entorno para verificar que funcionara antes de
entregármelo. Las partes que dependían de la pantalla (la interfaz gráfica) las
probé yo en mi máquina Arch Linux y le reporté los resultados con capturas. El
desarrollo fue incremental: se dividió en partes y cada una se probó antes de
seguir.

---

## Fase 1 — Organización del repositorio y primeras versiones del protocolo

El proyecto arrancó organizando cómo publicar la especificación común en GitHub
y cómo estructurar el repositorio.

**Prompt:** *"esto es lo que tenemos actualmente como lo divido para pasarlo a
github como beta"*

**Prompt:** *"ya pero ese que te mande es para ponernos todos los grupos de
acuerdo, aparte es el repo donde estara mi juego"*

El asistente distinguió entre el repositorio común (la especificación que todos
los grupos comparten) y el repositorio de mi propio juego, y propuso una
estructura para cada uno.

**Prompts sobre el README y la estructura:**
- *"dame un ejemplo para llenar el readme"*
- *"haz un readme con primero un resumen de que trata el proyecto luego pon la
  lista de los grupos y luego explicando cada seccion del repo"*
- *"no enfatices en que es opcional..."*
- *"Quita las notas de compatibilidad y lo de la ultima columna"*


En este ida y vuelta se fue afinando el README hasta dejarlo claro y sin relleno.

**Prompt sobre cómo organizar el protocolo:** *"lo de conformidad no me gusta el
nombre... yo pensaba crear una carpeta protocolo y crear un PRFC-CC8-2026.md y
meter ahi todo lo del protocolo. pero si lo secciono como dice acuerdos todo lo
tendria regado en .md no ?"*

Se decidió un documento único `PRFC-CC8-2026.md` con todo el protocolo, más una
carpeta de "acuerdos" (tipo ADR, registros de decisiones) y otra de ejemplos.

**Prompt clave — verificación contra el catedrático:** *"este es el pdf con las
especificaciones que nos dio el catedratico verifica que lo que tengamos como
prfc concuerde con lo que pide el documento"*

El asistente comparó el protocolo con las especificaciones oficiales y ajustó las
diferencias.

---

## Fase 2 — Evolución del protocolo a binario (v3.0)

Esta fase definió el formato técnico del protocolo, que es lo que permite que los
13 grupos interoperen.

**Prompt:** *"no seria una mejor idea pasar de json a bits ?"*

**Prompt:** *"pero el catedratico nos recomendo que lo hicieramos en bits"*

El asistente explicó las tres alternativas de serialización y sus riesgos de
interoperabilidad:

**Prompt:** *"dame un ejemplo de como quedaria un paquete enviado para moverse
utilizando json otro messagePack y otro binario a mano"*

Comparó los tres formatos con ejemplos concretos, mostrando cómo el mismo mensaje
de movimiento se ve en cada uno. Con esa información elegí el binario a mano.

**Decisión sobre el movimiento — Prompt:** *"donde el documento del catedratico
describe que hay movimientos en diagonal?"*

**Prompt:** *"es mejor que no hayan diagonales si el catedratico no lo especifico
para no complicar lo que hacemos no ?"*

El asistente confirmó que el reglamento no menciona diagonales, y se decidió el
movimiento en **cuatro direcciones** (arriba, abajo, izquierda, derecha), que es
más simple de acordar entre 13 implementaciones distintas.

**Prompt:** *"si hazlo asi dame el prfc actualizado utilizando bits, haz solo el
prfc"*

Se produjo el `PRFC-CC8-2026-v3.md`: el protocolo completo en formato binario,
big-endian, con framing por prefijo de longitud sobre TCP, coordenadas como
enteros (× 100), y movimiento en cuatro direcciones. Este documento fue después
adoptado por la clase como estándar.

**Prompts de aclaración del formato:**
- *"que es eso de u8"* → el asistente explicó los tipos base (u8, u16, i32, etc.).
- *"dame un resumen de como quedaron los movimientos en bits..."*
- *"dame un resumen de la estructura y creacion del mapa"*

La **prueba de oro** del protocolo quedó definida así: un mensaje `INPUT` de P07
hacia arriba debe dar exactamente los bytes `11 03 00 07 01`. Si una
implementación produce esos cinco bytes, puede interoperar.

---

## Fase 3 — Implementación del juego en Go, en 6 partes

**Prompt:** *"ahora haz el juego en base a ese prfc dividelo en partes para ir
implementandolo y yo ir probandolo en cada parte"*

**Prompt (elección de lenguaje y método):**
> P: ¿En qué lenguaje querés que implemente el juego?
> R: en go
> P: Para la Parte 1, ¿cómo preferís recibirla?
> R: codigo con logs y yo lo ejecuto aqui

Se acordó implementar en Go, dividido en partes, con código que imprime logs para
que yo lo ejecutara y verificara en cada paso.

### Parte 1 — El protocolo binario

La serialización de mensajes a bytes y de vuelta. El corazón es el codec. Por
ejemplo, así se serializa el mensaje de fin de partida:

```go
case GameOver:
    w.u8(TypeGameOver)   // 0x29
    w.u8(Version)        // 0x03
    w.u16(m.WinnerID)
    w.str(m.WinnerName)  // u8 de longitud + bytes UTF-8
    w.u8(m.Reason)
```

**Resultado que verifiqué:** la prueba de oro dio `11 03 00 07 01`, y los 17
tipos de mensaje pasaron ida y vuelta correctamente.

```go
fmt.Printf("   bytes obtenidos : %s\n", hex(body))
fmt.Printf("   bytes esperados : %s\n", hex(esperado))
if bytes.Equal(body, esperado) {
    fmt.Println("   ✓ COINCIDEN — este proyecto puede interoperar")
}
```

### Parte 2 — El motor de reglas

La lógica del juego sin red: movimiento, tomar y robar la bandera, la victoria. El
movimiento en cuatro direcciones se ve así:

```go
switch p.Direction {
case DirUp:
    p.Y -= step
case DirDown:
    p.Y += step
case DirLeft:
    p.X -= step
case DirRight:
    p.X += step
}
```

**Resultado que verifiqué:** una partida simulada en ASCII, con Ana tomando la
bandera, Beto robándosela, y Beto ganando al salir del círculo.

### Parte 3 — El servidor TCP

Acepta conexiones, arma el lobby, corre la cuenta regresiva y difunde el estado.

**Resultado que verifiqué (con captura):** tres jugadores reales (Ana, Beto,
Erick) conectados, la partida corriendo, y el `GAME_OVER` llegando a todos.

### Parte 4 — El descubrimiento por UDP

Permite encontrar servidores en la red sin escribir la IP, con broadcast.

**Resultado que verifiqué (con captura):** el cliente encontró el servidor solo,
respondiendo por la red real (192.168.0.9).

Durante esta parte hubo un error de compilación por dos archivos llamados
`discovery.go` en carpetas distintas:

**Prompt:** *"dame los archivos correctos ordenados bien que le pusiste nombres
iguales a varios"*

El asistente empaquetó el proyecto completo con la estructura clara y explicó la
regla de Go: todos los archivos de una carpeta comparten el mismo `package`.

### Parte 5 — Cliente, bot y sonda de conformidad

Un cliente reutilizable, un bot que juega solo, y la herramienta `probe` que
verifica si un servidor cumple el protocolo (§35).

**Resultado que verifiqué (con captura):** la sonda dio **9/9** contra mi
servidor. También capturé sin querer el manejo de una desconexión con la bandera
cayendo (`DROPPED`), que funcionó según el protocolo.

### Parte 6 — La interfaz gráfica (Ebitengine)

**Prompt:** *"si"* (para continuar con la interfaz)

La parte visual: dibuja el círculo, los jugadores, la bandera, y traduce el
teclado a direcciones. Esta fue la única parte que el asistente no pudo ejecutar
(su entorno no tiene pantalla); verificó la sintaxis y las llamadas a la API
contra la documentación, y la probé yo en mi máquina.

---

## Fase 4 — Refinamiento del juego

Con el juego funcionando, siguieron una serie de mejoras pedidas a partir del uso
real.

### Inicio manual de la partida

**Prompt:** *"en lugar que espere un numero de jugadores mejor un comando start
inicie la partida y en la interfaz un boton start si aunque le pongamos maximo 100
si hay 4 la podamos iniciar"*

Hubo una discusión importante sobre si esto requería tocar el protocolo común:

**Prompt:** *"no ya habia un game started en el protocolo ?"*

**Prompt:** *"y con el 29.4 GAME_COUNTDOWN no se podria iniciar ?"*

El asistente explicó que esos mensajes van del servidor al cliente, no al revés, y
que para un botón haría falta un mensaje nuevo. Decidí mantener el protocolo
intacto:

**Prompt:** *"agrega el start de lado del server no toques nada del protocolo"*

Quedó un comando `start` en la terminal del servidor, sin modificar el protocolo.

### Partidas en bucle

**Prompt:** *"ahorita cuando termina la partida el servidor se cierra
automaticamente no ?"*

**Prompt:** *"es mejor la 3"* (volver al lobby para jugar otra partida)

Se agregó que el servidor, al terminar una partida, vuelve al lobby en vez de
cerrarse, para poder jugar varias seguidas sin reconectar.

Esto trajo dos bugs que se resolvieron con depuración cuidadosa:

**Prompt:** *"no aun no funciona el start de lado de cliente si vuelve a la sala de
espera"* → el cliente cerraba la conexión al recibir `GAME_OVER`; se corrigió para
que siguiera escuchando.

**Prompt:** *"no pero vuelvo a escribir start pero no la vuelve a iniciar"* → había
una condición de carrera que se tragaba el segundo `start`; se eliminó el vaciado
del canal que la causaba.

### Pausa antes de volver al lobby

**Prompt:** *"si solo que un compañero se conecto pero no le aparecio el gano
salchipapa pero cuando se conecta a otros si le aparece"*

Diagnóstico: el servidor mandaba el regreso al lobby tan rápido después del
`GAME_OVER` que algunos clientes borraban el cartel de victoria antes de mostrarlo.
Se agregó una pausa configurable (`-postgame`), respetando el principio de "sé
estricto con lo que envías, tolerante con lo que recibís".

### Selección de servidores

**Prompt:** *"seria bueno que el cliente tenga una interfaz para mostrar los
servidores disponibles por si quiero conectarme a uno en especifico"*

Se agregó una lista de servidores que se actualiza sola, tanto en la terminal
(elegir por número) como en la ventana (elegir por clic).

Esto trajo un bug de Ebitengine (abrir dos ventanas en un proceso):

**Prompt:** *"[error de GLFW] sale cuando le doy click al servidor en la ventana"*

Se rediseñó para usar una sola ventana que cambia de pantalla internamente, en vez
de cerrar y reabrir.

### Herramientas de depuración para interoperabilidad

**Prompt:** *"no podriamos agregar logs de lado del server y cliente justo asi como
como me lo mostraste ahora, como envia el codigo y que codigo retorna"*

Se agregó un flag `-debug` que muestra cada mensaje en hex con el desglose byte por
byte, por ejemplo:

```
[servidor] ← RECIBE JOIN bytes:
hex: 10 03 03 41 6E 61
  10       -> tipo JOIN (0x10)
  03       -> versión del protocolo (3)
  03       -> name: largo = 3 (u8)
  41 6E 61 -> name = "Ana"
```

**Prompt:** *"no podrias agregar que cree un archivo con los logs... agregando el
flag -save"*

Se agregó un flag `-save` que guarda el log de cada partida en su propio archivo,
con fecha y hora en el nombre.

### Mostrar los nombres reales de los jugadores

**Prompt:** *"en los nombres de los jugadores muestra P01, P01 no se podria mostrar
el nombre de cada jugador ?"*

Diagnóstico: el `GAME_STATE` no incluye los nombres (para ahorrar bytes, según el
§29.6 del protocolo). Se corrigió el cliente para que guarde los nombres que llegan
en `GAME_STARTED` y `LOBBY_STATE` en una tabla `playerId → nombre`, y los use al
dibujar.

---

## Fase 5 — Cómo quedó implementado el backend (servidor y cliente)

Esta sección documenta en detalle cómo funciona la parte de red del proyecto: la
conexión TCP, el enmarcado de mensajes, el protocolo binario, el servidor, el
cliente reutilizable y el descubrimiento por broadcast UDP. Es el "backend" del
juego, es decir todo lo que ocurre por debajo de la interfaz gráfica.

La arquitectura está en capas y cada capa interna no sabe nada de las de arriba:

```
internal/protocol  →  solo el códec binario, no toca la red
internal/game      →  reglas puras del juego, sin red ni protocolo
internal/discovery →  cliente UDP: encuentra servidores por broadcast
internal/server    →  servidor TCP + respondedor de descubrimiento UDP
internal/client    →  cliente reutilizable (lo usan el bot y la interfaz)
```

**Prompt:** *"explicame cómo viaja un mensaje por el cable, de punta a punta, para
documentarlo"*

El asistente recorrió el camino completo de un mensaje: un struct se serializa a
bytes en `protocol.MarshalBinary`, se le antepone el prefijo de longitud en
`WriteFrame`, viaja por el socket TCP, y del otro lado `ReadFrame` lo recompone y
`UnmarshalBinary` lo vuelve struct. Todo lo demás (servidor, cliente) se construye
sobre esas cuatro funciones.

### 5.1 — El enmarcado TCP (framing por longitud)

**Prompt:** *"configura el enmarcado de TCP para que use un prefijo de longitud de
2 bytes big-endian antes de cada mensaje, como pide el §23.2"*

TCP es un flujo de bytes: no tiene noción de "mensajes". Si dos mensajes se mandan
seguidos, pueden llegar pegados o partidos. Por eso cada mensaje viaja con un
prefijo de longitud (u16 big-endian) que dice cuántos bytes ocupa el cuerpo. Así
el receptor sabe exactamente dónde termina uno y empieza el otro:

```go
// WriteFrame escribe un cuerpo con su prefijo de longitud u16.
func WriteFrame(w io.Writer, body []byte) error {
    if len(body) > 0xFFFF {
        return fmt.Errorf("mensaje de %d bytes excede el máximo de 65535", len(body))
    }
    var hdr [2]byte
    binary.BigEndian.PutUint16(hdr[:], uint16(len(body)))
    if _, err := w.Write(hdr[:]); err != nil {
        return err
    }
    _, err := w.Write(body)
    return err
}

// ReadFrame lee un mensaje enmarcado: 2 bytes de longitud y luego el cuerpo.
// Usa io.ReadFull porque TCP puede entregar la lectura partida.
func ReadFrame(r io.Reader) ([]byte, error) {
    var hdr [2]byte
    if _, err := io.ReadFull(r, hdr[:]); err != nil {
        return nil, err
    }
    n := binary.BigEndian.Uint16(hdr[:])
    body := make([]byte, n)
    if _, err := io.ReadFull(r, body); err != nil {
        return nil, err
    }
    return body, nil
}
```

El detalle clave es `io.ReadFull`: garantiza que se lean exactamente los bytes
esperados aunque el sistema operativo entregue la lectura en varios pedazos.

### 5.2 — El códec binario

**Prompt:** *"configura los tipos base del protocolo (u8, u16, u32, i32, str) con
helpers de lectura y escritura, para que el resto del códec se lea como la tabla
del documento"*

El códec usa dos ayudantes: un `writer` que va acumulando bytes y un `reader` que
los va consumiendo. Las coordenadas del mundo son `float64`, pero por el cable
viajan como enteros escalados ×100 (§24); el códec hace esa conversión de forma
transparente:

```go
// coord convierte una coordenada de mundo al i32 escalado por 100 (§24).
func coord(v float64) int32 { return int32(math.Round(v * 100)) }

func (r *reader) coord() float64 { return float64(r.i32()) / 100 }
```

**Prompt:** *"configura el reader para que, si una lectura se pasa del final del
buffer, marque el error una sola vez y todas las siguientes lo respeten — así
basta comprobarlo al final"*

Ese patrón evita comprobar el error después de cada campo: el `reader` guarda un
`err` "pegajoso" y solo se revisa una vez tras decodificar el mensaje completo:

```go
type reader struct {
    b   []byte
    i   int
    err error
}

func (r *reader) u16() uint16 {
    if r.err != nil || r.i+2 > len(r.b) {
        r.err = ErrShort
        return 0
    }
    v := binary.BigEndian.Uint16(r.b[r.i:])
    r.i += 2
    return v
}
```

`MarshalBinary` y `UnmarshalBinary` son un `switch` sobre el tipo de mensaje. Todo
mensaje empieza con dos bytes: el tipo y la versión. Al decodificar, la versión se
valida antes de leer el resto (§32):

```go
func UnmarshalBinary(body []byte) (any, error) {
    if len(body) < 2 {
        return nil, ErrShort
    }
    t := body[0]
    if body[1] != Version { // Version = 3
        return nil, ErrVersion
    }
    r := &reader{b: body, i: 2}
    // ... switch t { ... }
}
```

**La prueba de oro** sigue siendo el ancla de interoperabilidad: un `INPUT` de P07
hacia arriba debe producir exactamente `11 03 00 07 01`. Si el códec de cualquier
grupo genera esos cinco bytes, puede interoperar con el resto de la clase.

### 5.3 — El servidor TCP

El servidor es el árbitro de la partida. No juega (§4): no tiene entidad en el
mapa, solo coordina. Toda la lógica del juego vive en `internal/game`; este
paquete solo agrega la red.

**Prompt:** *"configura el servidor para que use una goroutine por conexión y
proteja todo el estado compartido con un único mutex"*

Cada cliente que se conecta corre en su propia goroutine (`handleConn`), que lee
mensajes en un bucle. Todo el estado compartido (la lista de clientes, el mundo,
el estado de la partida) está detrás de un solo mutex `s.mu`:

```go
func (s *Server) handleConn(conn net.Conn) {
    defer conn.Close()
    c := &client{conn: conn}
    for {
        body, err := protocol.ReadFrame(conn)
        if err != nil {
            break
        }
        msg, derr := protocol.UnmarshalBinary(body)
        if derr != nil {
            c.send(protocol.Error{Code: errorCode(derr), Description: derr.Error()})
            if derr == protocol.ErrVersion {
                break // nada de lo que siga servirá
            }
            continue
        }
        if stop := s.dispatch(c, msg); stop {
            break
        }
    }
    s.disconnect(c)
}
```

`dispatch` reparte cada mensaje al manejador que le toca. Un principio del
protocolo es "sé estricto con lo que envías, tolerante con lo que recibís": si
llega un mensaje que solo va del servidor al cliente, se responde con un ERROR en
vez de cerrar la conexión:

```go
func (s *Server) dispatch(c *client, msg any) bool {
    switch m := msg.(type) {
    case protocol.Join:
        return !s.handleJoin(c, m)
    case protocol.Input:
        s.handleInput(c, m)
    case protocol.Interact:
        s.handleInteract(c, m)
    case protocol.Leave:
        return true
    default:
        c.send(protocol.Error{Code: protocol.ErrInvalidMessage,
            Description: "este mensaje no va del cliente al servidor"})
    }
    return false
}
```

**Prompt:** *"configura el JOIN para que valide el nombre, rechace si la partida ya
empezó o está llena, y asigne un playerId incremental"*

`handleJoin` es la puerta de entrada. Valida el nombre, comprueba el estado y el
cupo, asigna un ID nuevo, agrega el jugador al mundo y difunde el lobby:

```go
func (s *Server) handleJoin(c *client, m protocol.Join) bool {
    name := strings.TrimSpace(m.Name)
    if name == "" || len([]rune(name)) > 20 {
        c.send(protocol.JoinRejected{Reason: protocol.ReasonInvalidName})
        return false
    }
    s.mu.Lock()
    if s.state != protocol.StateWaiting {
        s.mu.Unlock()
        c.send(protocol.JoinRejected{Reason: protocol.ReasonGameAlreadyStarted})
        return false
    }
    if len(s.clients) >= s.opt.MaxPlayers {
        s.mu.Unlock()
        c.send(protocol.JoinRejected{Reason: protocol.ReasonGameFull})
        return false
    }
    s.nextID++
    id := s.nextID
    c.playerID, c.name = id, name
    s.clients[id] = c
    s.world.AddPlayer(id, name)
    s.mu.Unlock()
    c.send(protocol.JoinAccepted{PlayerID: id, GameID: s.opt.GameID})
    // ... difundir LOBBY_STATE ...
}
```

**Prompt:** *"configura el ciclo de partidas en bucle: cuando una termina, el
servidor vuelve al lobby y espera otro 'start', sin cerrar el proceso ni obligar a
reconectar"*

El corazón del servidor es `Run`: un bucle que espera la orden de inicio, corre la
cuenta regresiva, corre la partida, muestra el resultado un rato y vuelve al
lobby. La señal de inicio va por un canal con capacidad 1 (`startCh`), lo que
evita perder un 'start' escrito durante la partida anterior:

```go
func (s *Server) Run() {
    go s.acceptLoop()
    for {
        select {
        case <-s.startCh:
        case <-s.done:
            return
        }
        s.runCountdown()
        s.runGame()
        // pausa para que todos vean el GAME_OVER, luego:
        s.volverAlLobby()
    }
}
```

**Prompt:** *"configura el ciclo de juego (tick) para que primero difunda los
eventos y después el estado, en el orden del §29.11"*

`runGame` abre un `time.Ticker` al intervalo de la config y llama a `tick()` en
cada pulso. `tick()` toma el mutex, avanza el mundo un ciclo y difunde: primero
los eventos puntuales (tomó/robó la bandera), después el `GAME_STATE`, y si hubo
ganador, el `GAME_OVER`:

```go
func (s *Server) tick() bool {
    s.mu.Lock()
    defer s.mu.Unlock()

    interacts := s.pendingInteract
    s.pendingInteract = map[uint16]bool{}
    ev := s.world.Step(interacts)

    // §29.11: primero los eventos, después el estado.
    for _, id := range ev.PickedUp {
        s.broadcastLocked(protocol.FlagPickedUp{Tick: s.world.Tick, PlayerID: id})
    }
    for _, st := range ev.Stolen {
        s.broadcastLocked(protocol.FlagStolen{Tick: s.world.Tick,
            PreviousCarrierID: st.Previous, NewCarrierID: st.New})
    }
    s.broadcastLocked(protocol.GameState{
        Tick: s.world.Tick, Flag: s.flagLocked(), Players: s.playersCompactLocked(),
    })
    if ev.Winner != 0 {
        s.state = protocol.StateFinished
        s.broadcastLocked(protocol.GameOver{WinnerID: ev.Winner,
            WinnerName: s.nombreLocked(ev.Winner), Reason: 0x01})
        return true
    }
    return false
}
```

Las interacciones de un ciclo se juntan en `pendingInteract` y se consumen de
golpe al empezar el `tick`: así varios toques de la misma tecla en un ciclo cuentan
como uno solo (§12). La difusión (`broadcastLocked`) recorre a todos los clientes y
les escribe el mismo mensaje ya serializado.

### 5.4 — El cliente reutilizable

El cliente está pensado para que lo use cualquier capa de arriba: el bot y la
interfaz gráfica. La idea central es que la red corre en su propia goroutine y
mantiene el último estado recibido; quien lo usa solo llama a `Snapshot()` para
ver el estado y a `Mover()` / `Interactuar()` para actuar. No tiene que saber nada
de bytes ni de sockets.

**Prompt:** *"configura el cliente para que la red corra en una goroutine aparte y
el resto del programa solo lea un Snapshot inmutable, sin candados"*

`Conectar` abre el socket, manda el JOIN y espera (hasta 5 s) la respuesta. Si el
servidor acepta, arranca la goroutine `leer` que procesa mensajes para siempre:

```go
func Conectar(addr, nombre string, ev Eventos) (*Cliente, error) {
    conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
    if err != nil {
        return nil, fmt.Errorf("no se pudo conectar a %s: %w", addr, err)
    }
    c := &Cliente{conn: conn, ev: ev, cerrado: make(chan struct{})}
    c.snap.Conectado = true
    c.snap.Estado = protocol.StateWaiting
    if err := c.enviar(protocol.Join{Name: nombre}); err != nil {
        conn.Close()
        return nil, err
    }
    aceptado := make(chan error, 1)
    go c.leer(aceptado)
    select {
    case err := <-aceptado:
        if err != nil {
            c.Cerrar()
            return nil, err
        }
        return c, nil
    case <-time.After(5 * time.Second):
        c.Cerrar()
        return nil, fmt.Errorf("el servidor no respondió al JOIN")
    }
}
```

El `Snapshot` es un valor: `Snapshot()` copia los slices y el mapa de nombres para
que quien lo lea no comparta memoria con la goroutine de red. Por eso se puede
leer desde la goroutine de dibujo de Ebitengine sin candados.

**Prompt:** *"configura el cliente para que descarte los GAME_STATE que lleguen
fuera de orden (§31)"*

UDP no, pero incluso sobre TCP un `GAME_STATE` viejo podría procesarse tarde. El
cliente lleva `lastTick` y descarta cualquier estado con un tick menor o igual al
último visto:

```go
case protocol.GameState:
    if m.Tick <= c.lastTick { // §31: ignorar estados viejos
        continue
    }
    c.lastTick = m.Tick
    c.mu.Lock()
    c.snap.Tick, c.snap.Jugadores, c.snap.Bandera = m.Tick, m.Players, m.Flag
    c.mu.Unlock()
```

**Prompt:** *"configura el cliente para que NO cierre la conexión al recibir
GAME_OVER, sino que siga escuchando por si empieza otra partida"*

Este fue uno de los arreglos importantes: al terminar la partida, el cliente no
cierra; guarda el ganador y sigue escuchando, porque el servidor le mandará un
`LOBBY_STATE` para la siguiente. Y como el `GAME_STATE` no trae nombres (§29.6), el
cliente los guarda de los mensajes que sí los traen (`GAME_STARTED`,
`LOBBY_STATE`) en una tabla `playerId → nombre`:

```go
case protocol.GameOver:
    c.mu.Lock()
    c.snap.Estado = protocol.StateFinished
    c.snap.GanadorID, c.snap.Ganador = m.WinnerID, m.WinnerName
    c.mu.Unlock()
    if c.ev.AlTerminar != nil {
        c.ev.AlTerminar(m.WinnerID, m.WinnerName)
    }
    // No cerramos: el servidor sigue vivo y nos mandará un LOBBY_STATE
    // para la próxima partida. Seguimos escuchando.
```

### 5.5 — El descubrimiento por broadcast UDP

Para no tener que escribir la IP del servidor a mano, el cliente puede encontrar
las partidas de la red con un broadcast UDP (§19).

**Prompt:** *"configura el cliente de descubrimiento para que mande un
DISCOVER_REQUEST por broadcast y junte las respuestas durante un tiempo dado,
evitando duplicados por gameId"*

El cliente (`internal/discovery`) abre un socket UDP, manda el pedido a la
dirección de broadcast (y también a loopback, porque en Linux el broadcast no
siempre le llega a un servidor de la misma máquina), y recoge respuestas hasta que
vence el plazo. La IP del servidor se toma del datagrama recibido, no del mensaje
(§27.2), porque un servidor con varias interfaces no sabe cuál ve el cliente:

```go
func Buscar(port int, esperar time.Duration) ([]Servidor, error) {
    conn, err := net.ListenUDP("udp4", &net.UDPAddr{Port: 0})
    if err != nil {
        return nil, fmt.Errorf("no se pudo abrir el socket UDP: %w", err)
    }
    defer conn.Close()
    req, _ := protocol.MarshalBinary(protocol.DiscoverRequest{})

    // Broadcast global y loopback: cubre el caso de probar en una sola máquina.
    destinos := []*net.UDPAddr{
        {IP: net.IPv4bcast, Port: port},
        {IP: net.IPv4(127, 0, 0, 1), Port: port},
    }
    for _, d := range destinos {
        conn.WriteToUDP(req, d)
    }

    encontrados := map[uint16]Servidor{}
    conn.SetReadDeadline(time.Now().Add(esperar))
    buf := make([]byte, 2048)
    for {
        n, from, err := conn.ReadFromUDP(buf)
        if err != nil {
            break // venció el plazo
        }
        msg, err := protocol.UnmarshalBinary(buf[:n])
        if err != nil {
            continue
        }
        resp, ok := msg.(protocol.DiscoverResponse)
        if !ok {
            continue
        }
        encontrados[resp.GameID] = Servidor{DiscoverResponse: resp, IP: from.IP}
    }
    // ... devolver la lista ...
}
```

**Prompt:** *"configura el servidor para que responda los DISCOVER_REQUEST por UDP,
pero solo mientras acepta jugadores (estado WAITING)"*

Del lado del servidor (`internal/server/discovery.go`), un socket UDP escucha los
pedidos y responde con un `DISCOVER_RESPONSE` directo al remitente — pero solo si
está en `StateWaiting`, porque una partida ya empezada no admite jugadores nuevos:

```go
func (s *Server) discoveryLoop() {
    buf := make([]byte, 2048)
    for {
        n, from, err := s.udp.ReadFromUDP(buf)
        if err != nil {
            return
        }
        msg, derr := protocol.UnmarshalBinary(buf[:n])
        if derr != nil {
            continue
        }
        if _, ok := msg.(protocol.DiscoverRequest); !ok {
            continue
        }
        s.mu.Lock()
        waiting := s.state == protocol.StateWaiting
        resp := protocol.DiscoverResponse{
            GameID: s.opt.GameID, ServerName: s.opt.ServerName,
            TCPPort: uint16(s.tcpPortLocked()), State: s.state,
            PlayerCount: uint16(len(s.clients)), MaximumPlayers: uint16(s.opt.MaxPlayers),
        }
        s.mu.Unlock()
        if !waiting {
            continue // §19: solo responden los que aceptan jugadores
        }
        body, _ := protocol.MarshalBinary(resp)
        s.udp.WriteToUDP(body, from) // respuesta directa al remitente (§27.2)
    }
}
```

Un `Escáner` en `internal/discovery` repite este `Buscar` en segundo plano y
mantiene la última lista encontrada, para que la interfaz muestre servidores
apareciendo y desapareciendo en vivo.

### 5.6 — Herramientas de depuración del backend

**Prompt:** *"configura un flag -debug que imprima cada mensaje en hex con el
desglose byte por byte, y un flag -save que guarde ese log en un archivo por
partida"*

Para depurar la interoperabilidad con los otros grupos, tanto el servidor como el
cliente tienen un flag `-debug` que muestra cada mensaje en hexadecimal con su
desglose campo por campo (`protocol.Desglosar`), y un flag `-save` que vuelca ese
log a un archivo con fecha y hora. Ejemplo de la salida de un JOIN entrando al
servidor:

```
[servidor] ← RECIBE JOIN bytes:
hex: 10 03 03 41 6E 61
  10       -> tipo JOIN (0x10)
  03       -> versión del protocolo (3)
  03       -> name: largo = 3 (u8)
  41 6E 61 -> name = "Ana"
```

---

## Fase 6 — El bot de IA (`cmd/bot`)

La Parte 5 incluye un bot que juega solo, encima del cliente reutilizable de la
sección anterior. Esta fase documenta las mejoras que se le hicieron a su
estrategia. (El transcript literal de esta sesión está en `docs/conversacion-bot.md`.)

**Prompt:** *"se puede mejorar el bot?"*

El asistente revisó el bot y la lógica de victoria, y encontró un bug de estrategia
más varias optimizaciones. La bandera nace en el centro (0,0), dentro del círculo.
Por la regla de victoria (acuerdo 005), para ganar hace falta `enteredCircle ==
true`, que se resetea cada vez que se toma o roba la bandera. El bot original
siempre corría hacia afuera al tener la bandera: funcionaba si la tomaba fresca en
el centro, pero si se la robaba a otro jugador fuera del círculo, nunca ganaba.

**Prompt:** *"sí, hacé las 3"* (las tres mejoras propuestas)

**Prompt:** *"configura la re-entrada al círculo: si el bot tiene la bandera pero
no entró al círculo desde que la tomó, que vaya primero al centro y después salga"*

Se implementaron tres mejoras en `cmd/bot/main.go`:

1. **Re-entrada al círculo tras robar** (la correctiva): el bucle rastrea
   `entroDesdeToma` detectando el flanco de subida de `HasFlag` y reproduce la
   semántica del servidor. Si no cumplió el requisito, apunta primero al borde del
   círculo hacia adentro para re-entrar, y recién después sale a ganar.
2. **Lead pursuit** (anticipar al portador): la función `interceptar` proyecta el
   movimiento del portador usando su `Direction` y `PlayerSpeed`, apuntando a
   dónde estará en vez de a dónde está.
3. **Anti zig-zag** (histéresis): `direccionHacia` recibe la dirección actual y
   aplica una banda muerta (margen = 20), para no cambiar de eje por diferencias
   mínimas cerca del objetivo.

**Prompt:** *"están bien pero están muy pros, agrégales una flag para ponerlos
fácil / intermedio / avanzado"*

**Prompt:** *"configura una flag -nivel con tres presets que ajusten reacción,
anticipación, re-entrada, histéresis y ruido de puntería"*

Se agregó la flag `-nivel` con tres dificultades. Cada nivel ajusta un conjunto de
parámetros del struct `nivel`:

| Parámetro | facil | intermedio | avanzado |
|---|---|---|---|
| Reacción (periodo) | 400 ms | 150 ms | 100 ms |
| Anticipación al perseguir | 0 (va detrás) | 0.5 (media) | 1.0 (completa) |
| Re-entra al círculo (acuerdo 005) | ❌ | ✅ | ✅ |
| Histéresis anti zig-zag | ❌ | ✅ | ✅ |
| Ruido de puntería | ±110 | 0 | 0 |
| Distracción | 40% por vuelta | 0 | 0 |

**Prompt:** *"no, el fácil todavía está muy pro, bajale"*

**Prompt:** *"configura el nivel fácil para que reaccione más lento, con más ruido,
y agregale distracción: que a veces se quede quieto o se vaya en una dirección al
azar"*

La distracción es lo que más lo hace ver principiante: en cada decisión, con 40 %
de probabilidad el bot fácil se queda quieto o se va en una dirección aleatoria en
vez de ir por la bandera. El ruido solo afecta el objetivo del movimiento, no el de
interactuar, así que igual agarra la bandera si pasa cerca — solo se mueve torpe.

**Prompt:** *"que los bots no se desconecten cuando termine la partida, sino que
esperen por si empieza otra"*

**Prompt:** *"configura el bucle del bot para que al ver StateFinished no salga:
que avise una vez, resetee su estado interno y siga esperando la próxima partida"*

Antes el bot salía al recibir `GAME_OVER`. Se corrigió para que, al detectar
`StateFinished`, avise una vez, resetee su estado interno (`ultimaDir`,
`teniaBandera`, `entroDesdeToma`) y siga en el bucle. El único caso en que el bot
termina ahora es cuando la conexión se cierra de verdad (`c.Listo()`), es decir
servidor apagado o kick. El reset de `ultimaDir = 255` asegura que el primer
`Mover` de la nueva partida siempre se envíe.

---

## Reflexión sobre el uso de la IA

El asistente funcionó como un par de programación: proponía soluciones, las
compilaba y ejecutaba para verificarlas, y explicaba las decisiones de diseño. Mi
rol fue guiar las decisiones (elegir binario sobre JSON, cuatro direcciones sin
diagonales, no tocar el protocolo común, volver al lobby entre partidas) y probar
en mi máquina lo que el asistente no podía ejecutar (la interfaz gráfica y el juego
en red real).

Varios de los bugs más difíciles —la condición de carrera del `start`, el timing
del `GAME_OVER`, los nombres faltantes— se encontraron a partir de mis reportes de
lo que veía al usar el juego, y se resolvieron reproduciéndolos y verificando el
arreglo antes de darlo por bueno. La documentación del protocolo y las herramientas
de depuración (`probe`, `-debug`, `-save`) fueron clave para poder interoperar con
los otros grupos.
