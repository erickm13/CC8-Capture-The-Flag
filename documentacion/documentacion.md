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
