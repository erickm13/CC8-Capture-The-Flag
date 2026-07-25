# CC8 Capture The Flag — estructura del proyecto

Reemplazá tu carpeta con estos archivos, respetando exactamente las rutas.

```
CC8-Capture-The-Flag/
├── go.mod
├── cmd/
│   ├── demo/main.go       ← Parte 1: prueba del protocolo binario (bytes)
│   ├── demo2/main.go      ← Parte 2: prueba del motor de reglas (partida ASCII)
│   ├── server/main.go     ← Parte 3+4: arranca el servidor
│   ├── sonda/main.go      ← Parte 3+4: cliente de prueba
│   └── discover/main.go   ← Parte 4: busca servidores en la red
└── internal/
    ├── protocol/
    │   ├── messages.go    ← structs de los mensajes + constantes
    │   └── codec.go       ← serialización binaria
    ├── game/
    │   └── world.go       ← motor de reglas (sin red)
    ├── discovery/
    │   └── discovery.go   ← package discovery — el CLIENTE busca servidores
    └── server/
        ├── server.go      ← package server — el servidor TCP
        └── discovery.go   ← package server — el servidor RESPONDE el broadcast
```

## OJO con los dos archivos que se llaman igual

Hay dos `discovery.go`, en carpetas distintas y con package distinto:

- `internal/discovery/discovery.go`  →  primera línea: `package discovery`
- `internal/server/discovery.go`     →  primera línea: `package server`

La regla de Go: todos los archivos de una carpeta comparten el mismo `package`.
Como `internal/server/` ya tiene `server.go` con `package server`, el
`discovery.go` de esa carpeta TAMBIÉN es `package server`.

## Comandos (siempre desde la raíz, donde está go.mod)

```bash
# Parte 1: ver los bytes del protocolo
go run ./cmd/demo

# Parte 2: ver una partida simulada en ASCII
go run ./cmd/demo2

# Parte 3+4: levantar el servidor (una terminal)
go run ./cmd/server -autostart 3 -small

# Parte 4: buscar servidores en la red (otra terminal)
go run ./cmd/discover

# Parte 3+4: conectar un jugador de prueba (otra terminal)
#   con -addr usás una dirección fija; sin -addr lo busca por broadcast
go run ./cmd/sonda -name Ana
go run ./cmd/sonda -name Beto -addr 127.0.0.1:5000
```

Verificá dónde estás parado con `ls go.mod`: si aparece, estás en la raíz.
