# Aluguel de Energia Tron via API
## SDK Go por TronZap.com

[English](README.md) | [Español](README.es.md) | **[Português](README.pt-br.md)** | [Русский](README.ru.md)

[![Go Reference](https://pkg.go.dev/badge/github.com/tron-energy-market/tronzap-sdk-go.svg)](https://pkg.go.dev/github.com/tron-energy-market/tronzap-sdk-go)
[![Go Report Card](https://goreportcard.com/badge/github.com/tron-energy-market/tronzap-sdk-go)](https://goreportcard.com/report/github.com/tron-energy-market/tronzap-sdk-go)
[![CI](https://github.com/tron-energy-market/tronzap-sdk-go/actions/workflows/ci.yml/badge.svg)](https://github.com/tron-energy-market/tronzap-sdk-go/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

SDK oficial em Go para a API do TronZap.
Este SDK permite integrar facilmente os serviços TronZap para aluguel de energia TRON.

TronZap.com permite [comprar energia TRON](https://tronzap.com/), reduzindo significativamente as taxas nas transferências de USDT (TRC20).

👉 [Registre-se para obter uma chave API](https://tronzap.com) para começar a usar a API TronZap e integrá-la através do SDK.

- Site: https://tronzap.com
- Referência da API: https://docs.tronzap.com/
- Documentação do pacote: https://pkg.go.dev/github.com/tron-energy-market/tronzap-sdk-go

## Instalação

```bash
go get github.com/tron-energy-market/tronzap-sdk-go
```

## Requisitos

- Go 1.21 ou superior
- Sem dependências externas

## Início rápido

```go
package main

import (
	"context"
	"fmt"
	"log"

	tronzap "github.com/tron-energy-market/tronzap-sdk-go"
)

func main() {
	client := tronzap.NewClient("seu_api_token", "seu_api_secret")
	ctx := context.Background()

	balance, err := client.GetBalance(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("saldo: %s (depositar em %s)\n", balance.Balance, balance.Address)

	// Estimar quanta energia uma transferência USDT precisa e comprar exatamente isso.
	estimate, err := client.EstimateEnergy(ctx, tronzap.EstimateEnergyRequest{
		FromAddress: "TSenderAddress",
		ToAddress:   "TRecipientAddress",
	})
	if err != nil {
		log.Fatal(err)
	}

	tx, err := client.CreateEnergyTransaction(ctx, tronzap.EnergyTransactionRequest{
		Address:         "TRecipientAddress",
		Energy:          estimate.Energy,
		Duration:        1,
		ExternalID:      "order-42",
		ActivateAddress: true,
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("a transação %s custa %s e está com status %s\n", tx.ID, tx.Amount, tx.Status)
}
```

Um passo a passo executável de todas as operações está em
[`examples/basic`](examples/basic/main.go):

```bash
export TRONZAP_API_TOKEN=seu_api_token
export TRONZAP_API_SECRET=seu_api_secret
export TRONZAP_BASE_URL=api.tronzap.com   # opcional
go run ./examples/basic
```

Por padrão ele apenas lê e não gasta nada. Definir `TRONZAP_ALLOW_PURCHASES=1`
também exercita os endpoints que criam transações e verificações AML, que debitam
o saldo da conta. Consulte o comentário no início do arquivo para as demais
variáveis opcionais.

## Configuração

`NewClient` recebe as duas credenciais do seu painel: o token da API é enviado
como token bearer e o segredo assina o corpo de cada requisição. Todo o resto é
uma opção:

```go
client := tronzap.NewClient(apiToken, apiSecret,
	tronzap.WithBaseURL("api.tronzap.com"), // padrão: tronzap.DefaultBaseURL
	tronzap.WithTimeout(10*time.Second),    // padrão: tronzap.DefaultTimeout (30s)
	tronzap.WithHTTPClient(myClient),       // seu próprio transporte, proxy ou retentativas
	tronzap.WithUserAgent("my-app/1.0"),
)
```

`WithBaseURL` aceita tanto um domínio simples quanto uma URL completa: quando o
esquema está ausente, usa-se `https`, e a barra final é removida, portanto
`"api.tronzap.com"`, `"api.tronzap.com/"` e `"https://api.tronzap.com"` são
equivalentes. Informe um esquema explícito para evitar isso, por exemplo
`"http://localhost:8080"` contra um servidor local de testes.

Um `Client` é seguro para uso concorrente e não mantém estado global, então crie
um por conjunto de credenciais e compartilhe-o. Um cliente que você passe via
`WithHTTPClient` nunca é modificado: `WithTimeout` aplica seu prazo a uma cópia.

Todos os métodos que fazem requisições recebem um `context.Context` como primeiro
argumento, então prazos e cancelamento funcionam como de costume:

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

services, err := client.GetServices(ctx)
```

## Métodos disponíveis

| Método | Endpoint | Descrição |
|---|---|---|
| `GetServices(ctx)` | `/v1/services` | Serviços disponíveis e preços |
| `GetBalance(ctx)` | `/v1/balance` | Saldo atual da conta |
| `GetAddressInfo(ctx, address)` | `/v1/address-info` | Recursos do endereço (energy, bandwidth) e saldos (TRX, USDT) |
| `EstimateEnergy(ctx, req)` | `/v1/estimate-energy` | Energia que uma transferência precisa e seu custo |
| `Calculate(ctx, req)` | `/v1/calculate` | Calcular o preço de uma compra sem criar uma transação |
| `CreateEnergyTransaction(ctx, req)` | `/v1/transaction/new` | Comprar energia |
| `CreateBandwidthTransaction(ctx, req)` | `/v1/transaction/new` | Comprar bandwidth |
| `CreateResourceBundleTransaction(ctx, req)` | `/v1/transaction/new` | Comprar energia e bandwidth em uma única transação |
| `CreateAddressActivationTransaction(ctx, req)` | `/v1/transaction/new` | Ativar um endereço TRON |
| `CheckTransaction(ctx, req)` | `/v1/transaction/check` | Status de uma transação, por id ou id externo |
| `GetDirectRechargeInfo(ctx)` | `/v1/direct-recharge-info` | Endereço e tarifas da recarga direta |
| `GetAMLServices(ctx)` | `/v1/aml-checks` | Serviços AML e preços |
| `CreateAMLCheck(ctx, req)` | `/v1/aml-checks/new` | Iniciar uma verificação AML |
| `CheckAMLStatus(ctx, id)` | `/v1/aml-checks/check` | Status e resultado de uma verificação AML |
| `GetAMLHistory(ctx, req)` | `/v1/aml-checks/history` | Histórico paginado de verificações AML |
| `Do(ctx, endpoint, params, result)` | qualquer | Acesso direto a endpoints ainda não cobertos |

Os campos opcionais ficam em estruturas de requisição em vez de longas listas de
parâmetros, de modo que novos campos da API podem ser adicionados sem quebrar seu
código. Valores zero significam «usar o padrão da API»: `Duration` passa a ser
1 hora e a paginação do histórico AML usa a página 1 com 10 itens.

### Comprar recursos

```go
// Energia, opcionalmente ativando o endereço na mesma chamada.
tx, err := client.CreateEnergyTransaction(ctx, tronzap.EnergyTransactionRequest{
	Address:         "TRecipientAddress",
	Energy:          65000,
	Duration:        1,      // horas; 1 ou 24
	ExternalID:      "order-42",
	ActivateAddress: true,
})

// Bandwidth.
tx, err = client.CreateBandwidthTransaction(ctx, tronzap.BandwidthTransactionRequest{
	Address:    "TRecipientAddress",
	Bandwidth:  345,
	ExternalID: "bandwidth-1",
})

// Energia e bandwidth juntos, mais barato do que comprar separadamente.
tx, err = client.CreateResourceBundleTransaction(ctx, tronzap.ResourceBundleTransactionRequest{
	Address:    "TRecipientAddress",
	Energy:     65000,
	Bandwidth:  345,
	Duration:   1,
	ExternalID: "bundle-1",
})

// Apenas a ativação.
tx, err = client.CreateAddressActivationTransaction(ctx, tronzap.AddressActivationRequest{
	Address:    "TRecipientAddress",
	ExternalID: "activation-1",
})
```

### Acompanhar uma transação

Uma transação passa por `new` → `pending` → `success` ou `failed`:

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

### Verificação AML

```go
check, err := client.CreateAMLCheck(ctx, tronzap.AMLCheckRequest{
	Type:    tronzap.AMLTypeAddress, // ou tronzap.AMLTypeHash
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

`RiskScore` é um `*Number` porque a API o deixa em null até a verificação
terminar; verifique se não é nil antes de lê-lo.

## Tratamento de erros

Toda falha é um `error` comum de Go. Quatro tipos concretos carregam os detalhes,
e cada um corresponde às sentinelas do pacote via `errors.Is`, permitindo ramificar
com a precisão que você precisar:

| Tipo | Significado | Corresponde a |
|---|---|---|
| `*APIError` | A API respondeu com um `code` diferente de zero | `ErrAPI` |
| `*HTTPError` | Resposta não 2xx sem conteúdo útil da API | `ErrHTTP`, e ainda `ErrRateLimit` (429), `ErrUnauthorized` (401/403) ou `ErrServer` (5xx) |
| `*NetworkError` | Nenhuma resposta chegou | `ErrNetwork`, e ainda `ErrConnection`, `ErrTimeout` ou `ErrTLS` |
| `*InvalidResponseError` | Resposta 2xx que o SDK não conseguiu decodificar | `ErrInvalidResponse` |

Argumentos que o SDK rejeita antes de enviar qualquer coisa correspondem a
`ErrInvalidRequest`.

```go
var apiErr *tronzap.APIError
switch {
case errors.As(err, &apiErr):
	// Falha no nível da aplicação: o código diz exatamente o que ocorreu.
	switch apiErr.Code {
	case tronzap.CodeInvalidTronAddress:
		// apiErr.Key pode detalhar, p. ex. "invalid_tron_address.from_address"
		log.Println("endereço inválido:", apiErr.Key)
	case tronzap.CodeInsufficientFunds:
		log.Println("adicione saldo à conta")
	case tronzap.CodeAddressNotActivated:
		log.Println("ative o endereço primeiro")
	default:
		log.Printf("erro da api %d: %s (requisição %s)", apiErr.Code, apiErr.Message, apiErr.RequestID)
	}
case errors.Is(err, tronzap.ErrRateLimit):
	// Aguardar e tentar novamente.
case errors.Is(err, tronzap.ErrUnauthorized):
	// Token ou assinatura inválidos.
case errors.Is(err, tronzap.ErrTimeout), errors.Is(err, tronzap.ErrServer):
	// Transitório; seguro tentar novamente.
case errors.Is(err, tronzap.ErrNetwork):
	// Inalcançável.
}
```

`APIError.RequestID` é o identificador que a API atribui a cada requisição:
informe-o ao contatar o suporte.

Como um `NetworkError` envolve o erro de transporte subjacente, falhas de contexto
continuam detectáveis diretamente:

```go
if errors.Is(err, context.Canceled) { /* quem chamou desistiu */ }
if errors.Is(err, context.DeadlineExceeded) { /* o prazo expirou */ }
```

Um erro da API tem precedência sobre o status HTTP: a API informa algumas falhas
com status 2xx e outras com 4xx ou 5xx, portanto um conteúdo decodificável com
código diferente de zero é sempre reportado como `*APIError`, nunca como
`*HTTPError`.

### Códigos de erro da API

| Código | Constante | Descrição |
|------|----------|-------------|
| 1 | `CodeAuth` | Erro de autenticação: token da API ou assinatura inválidos |
| 2 | `CodeInvalidServiceOrParams` | Serviço ou parâmetros inválidos |
| 5 | `CodeWalletNotFound` | Carteira interna não encontrada. Contate o suporte. |
| 6 | `CodeInsufficientFunds` | Saldo insuficiente |
| 10 | `CodeInvalidTronAddress` | Endereço TRON inválido |
| 11 | `CodeInvalidEnergyAmount` | Quantidade de energia inválida |
| 12 | `CodeInvalidDuration` | Duração inválida |
| 20 | `CodeTransactionNotFound` | Transação/assinatura não encontrada |
| 21 | `CodeCannotStopSubscription` | Não é possível parar a assinatura |
| 24 | `CodeAddressNotActivated` | Endereço não ativado |
| 25 | `CodeAddressAlreadyActivated` | Endereço já ativado |
| 30 | `CodeAMLCheckNotFound` | Verificação AML não encontrada |
| 35 | `CodeServiceNotAvailable` | Serviço não disponível |
| 50 | `CodeInvalidBandwidthAmount` | Quantidade de bandwidth inválida |
| 500 | `CodeInternalServerError` | Erro interno do servidor: contate o suporte |

## Campos decimais e de data

A API codifica valores monetários como número JSON em algumas respostas e como
string JSON em outras, por isso quantias e preços usam o tipo `Number`, que
decodifica ambas as formas e oferece `Float64()` e `String()`. Datas usam `Time`,
que incorpora `time.Time`, aceita os vários formatos que a API emite e preserva o
texto original em `Raw`. Uma data não reconhecida deixa o tempo incorporado zerado
em vez de invalidar toda a resposta.

## Acessar endpoints não cobertos

`Do` assina e envia uma requisição para qualquer endpoint e decodifica o campo
`result` no valor que você fornecer, sendo assim a forma de usar recursos da API
que este SDK ainda não cobre:

```go
var result struct {
	Items []struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	} `json:"items"`
}
err := client.Do(ctx, "/v1/subscriptions/history", map[string]any{"page": 1}, &result)
```

## Testes

```bash
go test ./...
go test -race -cover ./...
go vet ./...
gofmt -l .
```

## Licença

Licença MIT (MIT). Consulte o [arquivo de licença](LICENSE) para mais informações.

## Suporte

Para suporte, entre em contato com [support@tronzap.com](mailto:support@tronzap.com).
