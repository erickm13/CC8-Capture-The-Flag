# Conversación — mejoras al bot (cmd/bot)

Fecha: 2026-07-26. Transcript de la sesión de trabajo sobre `cmd/bot/main.go`.

---

**Claude:** ¡Hola! 👋

Veo que estás trabajando en el proyecto CC8-Capture-The-Flag — la implementación en Go del protocolo PRFC-CC8-2026 v3.

¿En qué te puedo ayudar hoy? Por ejemplo:

- 🐛 Depurar algún problema (vi que hay commits recientes arreglando bugs con el envío de paquetes GAME_OVER)
- ✨ Implementar una nueva funcionalidad
- 🔍 Revisar o explicar alguna parte del código (protocolo, game engine, server, client, bot...)
- 🧪 Escribir o correr tests
- 📡 Trabajar en el bot de IA (Parte 5)

¿Qué tienes en mente?

---

**Usuario:** se puede mejorar el bot ?

**Claude:** Buena pregunta. Déjame revisar cómo está implementado el bot actualmente para darte una respuesta concreta.

> `Bash` — find cmd/bot e internal, listado de archivos go relacionados
> `Read` cmd/bot/main.go
> `Bash` — grep de Snapshot / Player / Config
> `Bash` — grep de Flag.X / RequireEnteredCircle en world.go

Sí, se puede mejorar bastante. Revisé el bot y la lógica de victoria del juego, y hay una mejora que es prácticamente un bug de estrategia, además de varias optimizaciones. Te las ordeno por impacto:

**1. Re-entrada al círculo tras robar (lo más importante)**

La bandera nace en el centro (0,0), dentro del círculo. Por la regla de victoria (acuerdo 005, world.go:293-304), para ganar necesitas `enteredCircle == true`, que se resetea cada vez que tomas/robas la bandera (giveFlag, línea 310-315). Solo queda en true si la agarraste dentro del círculo.

El problema está en `decidir` (main.go:118-127): cuando el bot tiene la bandera, siempre corre hacia afuera. Eso funciona si la tomó fresca en el centro, pero si se la roba a otro jugador fuera del círculo, `enteredCircle` es false → corre afuera → nunca gana, se queda dando vueltas para siempre.

El bot debería rastrear si entró al círculo desde que tiene la bandera, y si no, ir primero al centro y luego salir.

**2. Anticipar al portador al perseguir**

En main.go:128 persigue la posición actual del portador, que está en movimiento a PlayerSpeed. Con "lead pursuit" (apuntar a dónde estará) lo alcanza mucho antes en vez de ir siempre detrás.

**3. Oscilación en zig-zag**

`direccionHacia` elige el eje más lejano y solo re-manda Mover al cambiar de dirección. Cerca del objetivo produce zig-zag ineficiente. Se puede suavizar con una banda muerta para que no cambie de eje por diferencias mínimas.

**4. Interceptar en vez de perseguir**

Si el portador va hacia el borde para ganar, en lugar de seguirlo por detrás conviene cortarle el paso hacia el punto de salida más cercano.

La #1 es la que puede hacer que el bot pierda partidas que debería ganar. ¿Quieres que implemente las mejoras? Puedo hacer solo la #1 (la correctiva) o el paquete completo (#1–#3). ¿Cómo prefieres?

---

**Usuario:** si haz las 3

**Claude:** Voy a implementarlas. Primero reviso la API completa del Snapshot y Player para trabajar con datos correctos.

> `Read` client.go (Snapshot/Player), `Bash` grep GameStarted y constantes DirUp/DirDown/…, `Read` world.go (paso de movimiento)

Tengo todo lo que necesito. Convención de ejes confirmada (DirUp = Y-, DirDown = Y+, etc.). Implemento las tres mejoras en cmd/bot/main.go.

> `Edit` cmd/bot/main.go — bucle `jugar`: rastrear `teniaBandera` y `entroDesdeToma`
> `Edit` cmd/bot/main.go — `decidir` con re-entrada, `interceptar`, `velocidadDe`, `direccionHacia` con histéresis
> `Bash` — `go build ./... && go vet ./cmd/bot/ && echo OK` → OK

Compila y pasa go vet. Listo, implementé las tres mejoras en cmd/bot/main.go:

✅ **#1 — Re-entrada al círculo (la correctiva)**
- El bucle `jugar` ahora rastrea `entroDesdeToma`: detecta el flanco de subida de HasFlag y reproduce la semántica del servidor (giveFlag).
- `decidir`: si aún no cumplió el requisito (típico al robar la bandera fuera del círculo), primero apunta al punto más cercano de la frontera hacia adentro para re-entrar, y recién después sale a ganar.

✅ **#2 — Anticipar al portador (lead pursuit)**
- Nueva función `interceptar`: proyecta el movimiento del portador usando su Direction y PlayerSpeed, con el tiempo de vuelo como anticipación. Helper `velocidadDe` traduce las 4 direcciones a un vector.

✅ **#3 — Anti zig-zag (histéresis)**
- `direccionHacia` recibe la dirección actual y aplica una banda muerta (margen = 20): solo cambia de eje si el otro es claramente mayor o si ya quedó alineado en el eje actual.

Para probarlo en vivo, en dos terminales:

```bash
go run ./cmd/server -autostart 2 -small
go run ./cmd/bot -name Ana        # y en otra: go run ./cmd/bot -name Beto
```

El escenario donde más se nota la mejora es cuando dos bots se roban la bandera fuera del círculo: antes ninguno cerraba la partida; ahora el que la tiene vuelve a entrar y sale a ganar.

¿Quieres que agregue un test unitario para decidir/interceptar, o que lo deje así?

---

**Usuario:** si estan bien pero estan muy pros, agregales una flag para ponerlos facil intermedio avanzado

**Claude:** Buena idea. Voy a agregar una flag -nivel con tres dificultades. Cada nivel ajusta: anticipación al perseguir, si sabe re-entrar al círculo (acuerdo 005), histéresis anti zig-zag, velocidad de reacción y "puntería" (ruido). Reviso el main.go actual para editar con precisión.

> `Read` cmd/bot/main.go
> `Edit` — imports (`math/rand`, `strings`), tipo `nivel` y `nivelPorNombre`
> `Edit` — flag `-nivel` y parseo en main
> `Edit` — log de conexión + llamada `jugar(c, *name, n)`
> `Edit` — `jugar` usa `n.periodo`, `n.ruido`, `n.histeresis`
> `Edit` — `decidir` recibe `anticipacion` y `reentra`; `interceptar` escala por anticipación
> `Bash` — `go build ./... && go vet ./cmd/bot/ && echo OK` → OK

Compila y pasa go vet. Agregada la flag -nivel con tres presets:

| Parámetro | facil | intermedio | avanzado |
|---|---|---|---|
| Reacción (periodo) | 250 ms | 150 ms | 100 ms |
| Anticipación al perseguir | 0 (va detrás) | 0.5 (media) | 1.0 (completa) |
| Re-entra al círculo (acuerdo 005) | ❌ corre afuera siempre | ✅ | ✅ |
| Histéresis anti zig-zag | ❌ zig-zaguea | ✅ | ✅ |
| Ruido de puntería | ±45 | 0 | 0 |

Detalles de diseño:
- El ruido solo afecta el objetivo del movimiento, no el del interactuar, así que el bot fácil igual agarra la bandera cuando pasa cerca — solo se mueve torpe.
- Sin reentra, el fácil vuelve al comportamiento ingenuo (corre siempre hacia afuera), así que falla justo el caso de robar la bandera fuera del círculo.
- avanzado es el default, o sea el comportamiento que ya tenías.

Uso:

```bash
go run ./cmd/bot -name Ana -nivel facil
go run ./cmd/bot -name Beto -nivel intermedio
go run ./cmd/bot -name Caro -nivel avanzado      # default
```

Acepta variantes: fácil/easy, medio/medium, dificil/hard. Si pasás un nivel inválido, sale con un mensaje claro.

¿Querés que actualice la sección de comandos del CLAUDE.md con la nueva flag, o lo dejamos así?

---

**Usuario:** no el facil todavia esta muy pro bajale

**Claude:** Tienes razón. Le bajo el nivel al facil: reacción más lenta, más ruido, y le agrego distracción (a veces se mueve en dirección aleatoria o se queda quieto), que es lo que más lo hace ver principiante.

> `Edit` — nuevo campo `distraccion` en el tipo `nivel`
> `Edit` — presets: facil = 400 ms, ruido ±110, distracción 0.4; intermedio/avanzado con distracción 0
> `Edit` — aplica la distracción en el bucle (quieto o dirección al azar)
> `Bash` — `go build ./... && go vet ./cmd/bot/ && echo OK` → OK

Listo. Le bajé bastante el nivel al facil:

| Parámetro | antes | ahora |
|---|---|---|
| Reacción | 250 ms | 400 ms (mucho más lento) |
| Ruido de puntería | ±45 | ±110 |
| Distracción | — | 40% por vuelta |

La distracción es la clave: en cada decisión, con 40% de probabilidad el bot fácil o se queda quieto o se va en una dirección aleatoria en vez de ir hacia la bandera. Sumado a que reacciona cada 400 ms y con puntería de ±110, ahora juega claramente como principiante: deambula, se traba y pierde la bandera fácil.

`DirUp + rng.Intn(4)` da un valor entre 0x01 y 0x04, que cubre exactamente las 4 direcciones válidas. intermedio y avanzado quedan igual (distracción 0).

```bash
go run ./cmd/bot -name Torpe -nivel facil
go run ./cmd/bot -name Pro -nivel avanzado
```

Si todavía lo ves muy hábil, puedo subir la distracción (p. ej. 0.55) o hacer que a veces ni presione la tecla de agarrar. ¿Cómo lo ves?

---

**Usuario:** que los bot no se desconecten cuando termine la partida si no que esperen por si empieza otra

**Claude:** Buena idea. Necesito entender el ciclo de vida: qué hace el servidor tras StateFinished (¿vuelve a StateWaiting?) y qué señala c.Listo(). Déjame revisar.

> `Bash` — grep de Listo/StateFinished en client.go y de estados en server/*.go
> `Read` server.go (volverAlLobby)

El servidor mantiene la conexión y vuelve a StateWaiting. Entonces el bot solo necesita no salir de jugar al ver StateFinished, sino seguir en el bucle esperando la próxima partida. Solo debe salir cuando la conexión se cierra (c.Listo()).

> `Edit` cmd/bot/main.go — manejo de `StateFinished`: avisar una vez, resetear estado y seguir esperando
> `Read` — verificación de que no quedó `if != StateRunning` duplicado
> `Bash` — `go build ./... && go vet ./cmd/bot/ && echo OK` → OK

Listo. Ahora el bot no se desconecta al terminar la partida:

- Al detectar StateFinished, en vez de salir, avisa una vez ("partida terminada; espero la próxima..."), resetea su estado interno (ultimaDir, teniaBandera, entroDesdeToma) y sigue en el bucle esperando.
- El servidor, tras mostrar el GAME_OVER, hace volverAlLobby() → StateWaiting manteniendo la conexión. Cuando arranque otra partida (StateRunning), el bot retoma solo sin reconectar.
- El único caso en que jugar retorna sigue siendo c.Listo(), es decir cuando la conexión se cierra de verdad (servidor apagado o kick). Ahí sí imprime "listo" y termina el proceso.

El reset de ultimaDir = 255 asegura que, en la nueva partida, el primer Mover siempre se envíe (no se "traga" el movimiento por coincidir con la dirección anterior).

Para probar el ciclo completo, en el servidor podés escribir start de nuevo después de cada partida (o usar -autostart N) y verás que los mismos bots vuelven a jugar sin reconectarse.
