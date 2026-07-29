# Captura la Bandera — CC8
Repositorio donde se discutio el protocolo a utilizar: https://github.com/erickm13/CC8-Protocolo

Juego multijugador en red donde varios jugadores compiten por una única bandera.
Implementación en Go del protocolo **PRFC-CC8-2026 v3.0** (transporte binario),
pensada para interoperar con los proyectos de los demás grupos del curso.

## El juego en una frase

Hay una bandera en el centro de un círculo. Para ganar tenés que entrar al
círculo, tomar la bandera, y salir completamente del círculo llevándola. El
primero que lo logra gana. Cualquiera puede robarte la bandera si se te acerca.

---

## Cómo se juega

- **Moverse:** flechas del teclado o WASD (arriba, abajo, izquierda, derecha).
- **Tomar o robar la bandera:** barra espaciadora (o Enter) cuando estás cerca.
- **Ganar:** salí del círculo llevando la bandera.

No hay diagonales: el movimiento es en cuatro direcciones. No hay inmunidad: te
pueden robar la bandera apenas la tomás.

---

## Requisitos

- **Go 1.22 o superior.**
- Para la interfaz gráfica (Ebitengine), en Arch Linux hacen falta las librerías
  del sistema:

  ```bash
  sudo pacman -S libx11 libxcursor libxi libxinerama libxrandr libxxf86vm mesa
  ```

  La primera vez, Go descarga Ebitengine solo:

  ```bash
  go mod tidy
  ```

  El resto del proyecto (servidor, bots, herramientas) no necesita nada de esto:
  es librería estándar de Go.

Todos los comandos se ejecutan **desde la raíz del proyecto** (la carpeta que
tiene `go.mod`). Si dudás de dónde estás, `ls go.mod`: si aparece, estás bien.

---

## Inicio rápido: una partida en tu máquina

Abrí tres terminales.

**Terminal 1 — el servidor:**

```bash
go run ./cmd/server
```

**Terminal 2 — un jugador:**

```bash
go run ./cmd/juego -name Ana
```

**Terminal 3 — otro jugador:**

```bash
go run ./cmd/juego -name Beto
```

Cuando los dos jugadores estén conectados, volvé a la Terminal 1 (el servidor) y
escribí `start` y Enter. Empieza la cuenta regresiva y arranca la partida.

Cuando alguien gana, el servidor muestra el resultado unos segundos y vuelve solo
al lobby. Escribí `start` de nuevo para otra partida, sin que nadie se
reconecte. Para cerrar el servidor: `Ctrl-C`.

---

## Los programas

El proyecto tiene varios ejecutables en `cmd/`. Los dos que se usan para jugar
son `server` y `juego`; el resto son herramientas de prueba y demostración.

| Comando | Para qué sirve |
|---|---|
| `cmd/server` | El servidor. Hospeda la partida, mantiene el estado, no juega. |
| `cmd/juego` | El cliente con ventana gráfica. Es con lo que se juega. |
| `cmd/discover` | Lista los servidores disponibles en la red. |
| `cmd/bot` | Un jugador automático que juega solo (para pruebas). |
| `cmd/probe` | Verifica si un servidor cumple el protocolo (§35). |
| `cmd/sonda` | Cliente de prueba simple, sin interfaz. |
| `cmd/demo` | Muestra los bytes del protocolo (sin red). |
| `cmd/demo2` | Simula una partida en ASCII (sin red). |

---

## El servidor

```bash
go run ./cmd/server [opciones]
```

Arranca en el puerto TCP 5000 y responde el descubrimiento en el UDP 5001. Se
queda esperando en el lobby hasta que escribís `start`.

| Opción | Por defecto | Qué hace |
|---|---|---|
| `-name` | "Partida de prueba" | Nombre del servidor que ven los demás. |
| `-port` | 5000 | Puerto TCP de la partida. |
| `-discovery-port` | 5001 | Puerto UDP del descubrimiento. |
| `-max` | 100 | Máximo de jugadores. |
| `-autostart N` | 0 (manual) | Arranca solo al llegar a N jugadores. 0 = con `start`. |
| `-postgame N` | 5 | Segundos que se muestra el resultado antes de volver al lobby. |
| `-small` | no | Mundo chico y rápido, para probar. |
| `-debug` | no | Muestra cada mensaje en hex con desglose byte por byte. |
| `-save` | no | Guarda el log de cada partida en un archivo (carpeta `logs/`). |

**Comandos en la terminal del servidor:** escribí `start` para iniciar una
partida, o `salir` para cerrar.

La partida arranca con los jugadores que haya en ese momento, sin importar el
máximo: 4 de 100 es suficiente.

---

## El cliente (jugar)

```bash
go run ./cmd/juego -name TuNombre [opciones]
```

| Opción | Por defecto | Qué hace |
|---|---|---|
| `-name` | "jugador" | Tu nombre, visible para los demás. |
| `-addr` | (vacío) | Dirección del servidor. Vacío = elegir de una lista. |
| `-menu` | "terminal" | Cómo elegir servidor sin `-addr`: `terminal` o `ventana`. |
| `-discovery-port` | 5001 | Puerto UDP del descubrimiento. |
| `-debug` | no | Muestra cada mensaje en hex (llena la terminal; usar con cuidado). |
| `-save` | no | Guarda el log de la partida en un archivo. |

**Tres formas de conectarte a un servidor:**

1. **Elegir de una lista en la terminal** (por defecto). Muestra los servidores
   de la red, que se actualizan solos, y elegís por número:

   ```bash
   go run ./cmd/juego -name Ana
   ```

2. **Elegir de una lista en la ventana.** Abre una pantalla gráfica con la lista;
   hacés clic en el servidor:

   ```bash
   go run ./cmd/juego -name Ana -menu ventana
   ```

3. **Conexión directa por IP**, si ya sabés la dirección:

   ```bash
   go run ./cmd/juego -name Ana -addr 192.168.1.40:5000
   ```

---

## Descubrir servidores en la red

Para ver qué servidores hay sin conectarte:

```bash
go run ./cmd/discover
```

Lista cada servidor con su nombre, dirección, estado (esperando o jugando) y
cuántos jugadores tiene. Es el mismo descubrimiento que usa el cliente por
dentro.

---

## Jugar contra otros grupos (interoperabilidad)

El protocolo es común a los 13 grupos, así que tu cliente puede jugar en el
servidor de otro grupo y tu servidor acepta clientes de otros. Antes de conectar,
conviene verificar que el otro servidor cumple el protocolo:

```bash
go run ./cmd/probe -addr IP_DEL_OTRO_GRUPO:5000
```

La sonda ejecuta los pasos del §35 (JOIN, LOBBY, movimiento, tomar la bandera,
etc.) y dice si el servidor los pasa. Si algún paso falla, indica cuál.

### Depurar problemas de interoperabilidad

Cuando dos implementaciones no se entienden, activá `-debug` en ambos lados para
ver los bytes exactos que viajan:

```bash
go run ./cmd/server -debug
```

Cada mensaje se muestra con su hex y el desglose campo por campo:

```
[servidor] ← RECIBE JOIN bytes:
hex: 10 03 03 41 6E 61
  10       -> tipo JOIN (0x10)
  03       -> versión del protocolo (3)
  03       -> name: largo = 3 (u8)
  41 6E 61 -> name = "Ana"
```

Comparando los dos lados, los problemas típicos saltan a la vista: un entero en
el orden de bytes equivocado, una longitud leída como `u16` en vez de `u8`, o un
campo de más o de menos.

### Guardar los logs en archivo

Como una partida genera más de 150 mensajes en pocos segundos, el log en
pantalla es ilegible. Con `-save`, cada partida se guarda en su propio archivo,
con fecha y hora en el nombre:

```bash
go run ./cmd/server -save
```

Crea archivos como `logs/ctf-servidor-2026-07-26_15-04-05-partida01.log`. Un
archivo nuevo por cada partida. Funciona también del lado del cliente:

```bash
go run ./cmd/juego -name Ana -save
```

`-debug` (pantalla) y `-save` (archivo) son independientes: podés usar solo
`-save` para guardar sin llenar la terminal.

---

## Probar sin red

Dos programas muestran cómo funciona el juego sin abrir conexiones:

```bash
go run ./cmd/demo    # muestra los bytes del protocolo, mensaje por mensaje
go run ./cmd/demo2   # simula una partida completa dibujada en ASCII
```

Sirven para entender el protocolo y las reglas sin montar un servidor.

---

## Bots (jugadores automáticos)

Para rellenar una partida o probar sin gente:

```bash
go run ./cmd/bot -name Bot1
go run ./cmd/bot -name Bot2
```

Cada bot busca la bandera, la toma, y corre a salir del círculo. Aceptan las
mismas opciones de conexión (`-addr`, `-debug`, `-save`) que el cliente.

---

## Estructura del proyecto

```
cmd/          los ejecutables (server, juego, bot, probe, etc.)
internal/
  protocol/   los mensajes y su serialización binaria; el desglose de -debug
  game/       el motor de reglas (movimiento, bandera, victoria) sin red
  server/     el servidor TCP y el descubrimiento UDP
  client/     el cliente reutilizable que usa la interfaz
  discovery/  la búsqueda de servidores por broadcast
  ui/         la interfaz gráfica con Ebitengine
```

La separación es a propósito: `game/` no sabe de red, `ui/` no sabe de bytes, y
`protocol/` no sabe de reglas. Cada capa se puede probar por separado.

Ver `ESTRUCTURA.md` para el detalle archivo por archivo.

---

## Sobre el protocolo

El protocolo completo está en `PRFC-CC8-2026-v3.md`. Los puntos clave:

- **Transporte:** TCP para la partida (puerto 5000), UDP broadcast para el
  descubrimiento (puerto 5001).
- **Formato:** binario, big-endian, con un prefijo de longitud `u16` por mensaje.
- **Coordenadas:** enteros, en unidades de mundo × 100 (nada de decimales en el
  cable).
- **Movimiento:** cuatro direcciones, sin diagonales.
- **Versión:** el byte `3` viaja en cada mensaje.

La prueba de oro del protocolo: un `INPUT` de P07 hacia arriba debe dar
exactamente los bytes `11 03 00 07 01`. Si tu implementación produce esos cinco
bytes, puede interoperar.
