# Alquiler de Energía Tron vía API
## SDK Go por TronZap.com

[English](README.md) | **[Español](README.es.md)** | [Português](README.pt-br.md) | [Русский](README.ru.md)

[![Go Reference](https://pkg.go.dev/badge/github.com/tron-energy-market/tronzap-sdk-go.svg)](https://pkg.go.dev/github.com/tron-energy-market/tronzap-sdk-go)
[![Go Report Card](https://goreportcard.com/badge/github.com/tron-energy-market/tronzap-sdk-go)](https://goreportcard.com/report/github.com/tron-energy-market/tronzap-sdk-go)
[![CI](https://github.com/tron-energy-market/tronzap-sdk-go/actions/workflows/ci.yml/badge.svg)](https://github.com/tron-energy-market/tronzap-sdk-go/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

SDK oficial en Go para la API de TronZap.
Este SDK permite integrar fácilmente los servicios de TronZap para alquilar energía TRON.

TronZap.com permite [comprar energía TRON](https://tronzap.com/), reduciendo significativamente las comisiones en transferencias de USDT (TRC20).

👉 [Regístrate para obtener una clave API](https://tronzap.com) para comenzar a usar la API de TronZap e integrarla a través del SDK.

- Sitio web: https://tronzap.com
- Referencia de la API: https://docs.tronzap.com/
- Documentación del paquete: https://pkg.go.dev/github.com/tron-energy-market/tronzap-sdk-go

## Instalación

```bash
go get github.com/tron-energy-market/tronzap-sdk-go
```

## Requisitos

- Go 1.21 o superior
- Sin dependencias externas

## Inicio rápido

```go
package main

import (
	"context"
	"fmt"
	"log"

	tronzap "github.com/tron-energy-market/tronzap-sdk-go"
)

func main() {
	client := tronzap.NewClient("su_api_token", "su_api_secret")
	ctx := context.Background()

	balance, err := client.GetBalance(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("saldo: %s (depositar en %s)\n", balance.Balance, balance.Address)

	// Estimar cuánta energía necesita una transferencia USDT y comprar exactamente esa cantidad.
	estimate, err := client.EstimateEnergy(ctx, tronzap.EstimateEnergyRequest{
		FromAddress: "TSenderAddress",
		ToAddress:   "TRecipientAddress",
	})
	if err != nil {
		log.Fatal(err)
	}

	tx, err := client.CreateEnergyTransaction(ctx, tronzap.EnergyTransactionRequest{
		Address:         "TRecipientAddress",
		Energy:          estimate.Amount,
		Duration:        1,
		ExternalID:      "order-42",
		ActivateAddress: true,
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("la transacción %s cuesta %s y está en estado %s\n", tx.ID, tx.Amount, tx.Status)
}
```

Un recorrido ejecutable por todas las operaciones se encuentra en
[`examples/basic`](examples/basic/main.go):

```bash
export TRONZAP_API_TOKEN=su_api_token
export TRONZAP_API_SECRET=su_api_secret
export TRONZAP_BASE_URL=api.tronzap.com   # opcional
go run ./examples/basic
```

Por defecto solo lee y no gasta nada. Con `TRONZAP_ALLOW_PURCHASES=1` también se
ejercitan los endpoints que crean transacciones y verificaciones AML, que debitan
el saldo de la cuenta. Consulte el comentario al inicio del archivo para conocer
las demás variables opcionales.

## Configuración

`NewClient` recibe las dos credenciales de su panel: el token de la API se envía
como token bearer y el secreto firma el cuerpo de cada petición. Todo lo demás es
una opción:

```go
client := tronzap.NewClient(apiToken, apiSecret,
	tronzap.WithBaseURL("api.tronzap.com"), // por defecto tronzap.DefaultBaseURL
	tronzap.WithTimeout(10*time.Second),    // por defecto tronzap.DefaultTimeout (30s)
	tronzap.WithHTTPClient(myClient),       // su propio transporte, proxy o reintentos
	tronzap.WithUserAgent("my-app/1.0"),
)
```

`WithBaseURL` acepta tanto un dominio simple como una URL completa: si falta el
esquema se usa `https` y se elimina la barra final, por lo que
`"api.tronzap.com"`, `"api.tronzap.com/"` y `"https://api.tronzap.com"` son
equivalentes. Indique un esquema explícito para evitarlo, por ejemplo
`"http://localhost:8080"` contra un servidor local de pruebas.

Un `Client` es seguro para uso concurrente y no mantiene estado global, así que
cree uno por cada juego de credenciales y compártalo. Un cliente que usted pase
mediante `WithHTTPClient` nunca se modifica: `WithTimeout` aplica su plazo a una
copia.

Todos los métodos que realizan peticiones reciben un `context.Context` como primer
argumento, por lo que los plazos y la cancelación funcionan como es habitual:

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

services, err := client.GetServices(ctx)
```

## Métodos disponibles

| Método | Endpoint | Descripción |
|---|---|---|
| `GetServices(ctx)` | `/v1/services` | Servicios disponibles y precios |
| `GetBalance(ctx)` | `/v1/balance` | Saldo actual de la cuenta |
| `GetAddressInfo(ctx, address)` | `/v1/address-info` | Recursos de la dirección (energy, bandwidth) y saldos (TRX, USDT) |
| `EstimateEnergy(ctx, req)` | `/v1/estimate-energy` | Energía que necesita una transferencia y su coste |
| `Calculate(ctx, req)` | `/v1/calculate` | Calcular el precio de una compra sin crear una transacción |
| `CreateEnergyTransaction(ctx, req)` | `/v1/transaction/new` | Comprar energía |
| `CreateBandwidthTransaction(ctx, req)` | `/v1/transaction/new` | Comprar bandwidth |
| `CreateResourceBundleTransaction(ctx, req)` | `/v1/transaction/new` | Comprar energía y bandwidth en una sola transacción |
| `CreateAddressActivationTransaction(ctx, req)` | `/v1/transaction/new` | Activar una dirección TRON |
| `CheckTransaction(ctx, req)` | `/v1/transaction/check` | Estado de una transacción, por id o id externo |
| `GetDirectRechargeInfo(ctx)` | `/v1/direct-recharge-info` | Dirección y tarifas de la recarga directa |
| `GetAMLServices(ctx)` | `/v1/aml-checks` | Servicios AML y precios |
| `CreateAMLCheck(ctx, req)` | `/v1/aml-checks/new` | Iniciar una verificación AML |
| `CheckAMLStatus(ctx, id)` | `/v1/aml-checks/check` | Estado y resultado de una verificación AML |
| `GetAMLHistory(ctx, req)` | `/v1/aml-checks/history` | Historial paginado de verificaciones AML |
| `GetSubscriptions(ctx)` | `/v1/subscriptions` | Planes de suscripción y precios |
| `StartSubscription(ctx, req)` | `/v1/subscription/start` | Suscribir una dirección a un plan |
| `CheckSubscription(ctx, req)` | `/v1/subscription/check` | Estado de una suscripción, por id o id externo |
| `StopSubscription(ctx, req)` | `/v1/subscription/stop` | Detener una suscripción |
| `GetSubscriptionHistory(ctx, req)` | `/v1/subscriptions/history` | Historial paginado de suscripciones |
| `Do(ctx, endpoint, params, result)` | cualquiera | Acceso directo a endpoints aún no cubiertos |

Los campos opcionales viven en estructuras de petición en lugar de en largas
listas de parámetros, de modo que se pueden añadir campos nuevos de la API sin
romper su código. Los valores cero significan «usar el valor por defecto de la
API»: `Duration` pasa a ser 1 hora y la paginación de los historiales AML y de
suscripciones usa la página 1 con 10 elementos. La excepción es
`StartSubscriptionRequest`, donde un `DurationDays` o `TransactionsLimit` igual a
cero significa sin límite.

### Comprar recursos

```go
// Energía, activando opcionalmente la dirección en la misma llamada.
tx, err := client.CreateEnergyTransaction(ctx, tronzap.EnergyTransactionRequest{
	Address:         "TRecipientAddress",
	Energy:          65000,
	Duration:        1,      // horas; solo se admite 1
	ExternalID:      "order-42",
	ActivateAddress: true,
})

// Bandwidth.
tx, err = client.CreateBandwidthTransaction(ctx, tronzap.BandwidthTransactionRequest{
	Address:    "TRecipientAddress",
	Bandwidth:  345,
	ExternalID: "bandwidth-1",
})

// Energía y bandwidth juntos, más económico que comprarlos por separado.
tx, err = client.CreateResourceBundleTransaction(ctx, tronzap.ResourceBundleTransactionRequest{
	Address:    "TRecipientAddress",
	Energy:     65000,
	Bandwidth:  345,
	Duration:   1,
	ExternalID: "bundle-1",
})

// Solo la activación.
tx, err = client.CreateAddressActivationTransaction(ctx, tronzap.AddressActivationRequest{
	Address:    "TRecipientAddress",
	ExternalID: "activation-1",
})
```

### Seguir una transacción

Una transacción pasa por `new` → `pending` → `success` o `failed`:

```go
for {
	tx, err := client.CheckTransaction(ctx, tronzap.CheckTransactionRequest{ExternalID: "order-42"})
	if err != nil {
		return err
	}
	if tx.Status == tronzap.TransactionStatusSuccess || tx.Status == tronzap.TransactionStatusFailed {
		fmt.Println("finalizada como", tx.Status, "hash", tx.Hash)
		return nil
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(2 * time.Second):
	}
}
```

### Verificación AML

```go
check, err := client.CreateAMLCheck(ctx, tronzap.AMLCheckRequest{
	Type:    tronzap.AMLTypeAddress, // o tronzap.AMLTypeHash
	Network: "TRX",
	Address: "TAddressToScreen",
})
if err != nil {
	return err
}

result, err := client.CheckAMLStatus(ctx, check.ID)
if err != nil {
	return err
}
if result.Status == tronzap.AMLStatusCompleted {
	fmt.Println(result.RiskLevel, result.Blacklist, result.RiskFactors)
}
```

En una verificación por hash, `Address` es la dirección del destinatario de la
transacción, donde se recibieron los fondos, y `Direction` indica en qué lado
estás: `AMLDirectionDeposit` si los fondos llegaron a tu dirección (`Address` es
tu dirección), `AMLDirectionWithdrawal` si los enviaste tú (`Address` es la
dirección del destinatario externo). El riesgo se calcula para la contraparte: el
remitente en un deposit, el destinatario en un withdrawal. Si `Direction` está
vacío, el SDK envía `AMLDirectionDeposit`.

`RiskScore` es un `*Number` porque la API lo deja en null hasta que termina la
verificación; compruebe que no sea nil antes de leerlo.

### Suscripciones

Una suscripción mantiene una dirección abastecida de energía para cada transacción
hasta que se detiene o se agotan sus días o transacciones. Elija un plan de
`GetSubscriptions` y pase su `SubscriptionID`, como `"unlimited_energy"`, no su
`ID` numérico:

```go
plans, err := client.GetSubscriptions(ctx)
if err != nil {
	return err
}
for _, plan := range plans {
	fmt.Println(plan.SubscriptionID, plan.InitialPrice, plan.Price)
}

sub, err := client.StartSubscription(ctx, tronzap.StartSubscriptionRequest{
	SubscriptionID:    "unlimited_energy",
	Address:           "TRecipientAddress",
	DurationDays:      30, // 0 para no limitar el tiempo
	TransactionsLimit: 0,  // 0 para no limitar
	ExternalID:        "subscription-42",
})

sub, err = client.CheckSubscription(ctx, tronzap.SubscriptionRequest{ExternalID: "subscription-42"})

sub, err = client.StopSubscription(ctx, tronzap.SubscriptionRequest{ID: sub.ID})

history, err := client.GetSubscriptionHistory(ctx, tronzap.SubscriptionHistoryRequest{
	Status: tronzap.SubscriptionStatusActive,
})
```

Iniciar, consultar y detener devuelven la suscripción con sus `Params`; el
historial devuelve en su lugar los contadores de uso `TransactionsUsed`,
`EnergyUsed` y `TotalPrice`. Una suscripción con límite de transacciones no se
puede detener (`CodeCannotStopSubscription`).

## Gestión de errores

Todo fallo es un `error` de Go corriente. Cuatro tipos concretos aportan los
detalles, y cada uno coincide con los centinelas del paquete mediante
`errors.Is`, de forma que puede ramificar con la precisión que necesite:

| Tipo | Significado | Coincide con |
|---|---|---|
| `*APIError` | La API respondió con un `code` distinto de cero | `ErrAPI` |
| `*HTTPError` | Respuesta no 2xx sin contenido útil de la API | `ErrHTTP`, y además `ErrRateLimit` (429), `ErrUnauthorized` (401/403) o `ErrServer` (5xx) |
| `*NetworkError` | No llegó ninguna respuesta | `ErrNetwork`, y además `ErrConnection`, `ErrTimeout` o `ErrTLS` |
| `*InvalidResponseError` | Respuesta 2xx que el SDK no pudo decodificar | `ErrInvalidResponse` |

Los argumentos que el SDK rechaza antes de enviar nada coinciden con
`ErrInvalidRequest`.

```go
var apiErr *tronzap.APIError
switch {
case errors.As(err, &apiErr):
	// Fallo a nivel de aplicación: el código indica exactamente qué ocurrió.
	switch apiErr.Code {
	case tronzap.CodeInvalidTronAddress:
		// apiErr.Key puede precisarlo, p. ej. "invalid_tron_address.from_address"
		log.Println("dirección incorrecta:", apiErr.Key)
	case tronzap.CodeInsufficientFunds:
		log.Println("recargue la cuenta")
	case tronzap.CodeAddressNotActivated:
		log.Println("active primero la dirección")
	default:
		log.Printf("error de la api %d: %s (petición %s)", apiErr.Code, apiErr.Message, apiErr.RequestID)
	}
case errors.Is(err, tronzap.ErrRateLimit):
	// Esperar y reintentar.
case errors.Is(err, tronzap.ErrUnauthorized):
	// Token o firma incorrectos.
case errors.Is(err, tronzap.ErrTimeout), errors.Is(err, tronzap.ErrServer):
	// Transitorio; se puede reintentar.
case errors.Is(err, tronzap.ErrNetwork):
	// Inalcanzable.
}
```

`APIError.RequestID` es el identificador que la API asigna a cada petición:
indíquelo al contactar con soporte.

Como un `NetworkError` envuelve el error de transporte subyacente, los fallos de
contexto siguen siendo detectables directamente:

```go
if errors.Is(err, context.Canceled) { /* quien llamó desistió */ }
if errors.Is(err, context.DeadlineExceeded) { /* se agotó el plazo */ }
```

Un error de la API tiene prioridad sobre el estado HTTP: la API notifica algunos
fallos con un estado 2xx y otros con 4xx o 5xx, por lo que un contenido
decodificable con un código distinto de cero siempre se reporta como `*APIError`,
nunca como `*HTTPError`.

### Códigos de error de la API

| Código | Constante | Descripción |
|------|----------|-------------|
| 1 | `CodeAuth` | Error de autenticación: token de API o firma no válidos |
| 2 | `CodeInvalidServiceOrParams` | Servicio o parámetros no válidos |
| 5 | `CodeWalletNotFound` | Monedero interno no encontrado. Contacte con soporte. |
| 6 | `CodeInsufficientFunds` | Fondos insuficientes |
| 10 | `CodeInvalidTronAddress` | Dirección TRON no válida, o la dirección ya tiene una suscripción activa |
| 11 | `CodeInvalidEnergyAmount` | Cantidad de energía no válida |
| 12 | `CodeInvalidDuration` | Duración no válida |
| 20 | `CodeTransactionNotFound` | Transacción/suscripción no encontrada |
| 21 | `CodeCannotStopSubscription` | No se puede detener la suscripción, p. ej. tiene límite de transacciones |
| 24 | `CodeAddressNotActivated` | Dirección no activada |
| 25 | `CodeAddressAlreadyActivated` | Dirección ya activada |
| 30 | `CodeAMLCheckNotFound` | Verificación AML no encontrada |
| 35 | `CodeServiceNotAvailable` | Servicio no disponible |
| 50 | `CodeInvalidBandwidthAmount` | Cantidad de bandwidth no válida |
| 500 | `CodeInternalServerError` | Error interno del servidor: contacte con soporte |

## Campos decimales y de fecha

La API codifica los importes como número JSON en algunas respuestas y como cadena
JSON en otras, por lo que las cantidades y los precios usan el tipo `Number`, que
decodifica ambas formas y ofrece `Float64()` y `String()`. Las fechas usan `Time`,
que incorpora `time.Time`, acepta los distintos formatos que emite la API y
conserva el texto original en `Raw`. Una fecha no reconocida deja el tiempo
incorporado en cero en lugar de invalidar toda la respuesta.

## Acceder a endpoints no cubiertos

`Do` firma y envía una petición a cualquier endpoint y decodifica el campo
`result` en el valor que usted proporcione, que es la forma de usar funciones de
la API que este SDK todavía no cubre:

```go
var result struct {
	Items []struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	} `json:"items"`
}
err := client.Do(ctx, "/v1/new-endpoint", map[string]any{"page": 1}, &result)
```

## Pruebas

```bash
go test ./...
go test -race -cover ./...
go vet ./...
gofmt -l .
```

## Licencia

Licencia MIT (MIT). Consulte el [archivo de licencia](LICENSE) para más información.

## Soporte

Para soporte, contacte con [support@tronzap.com](mailto:support@tronzap.com).
