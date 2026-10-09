# Покупка энергии Tron через API
## Go SDK от TronZap.com

[English](README.md) | [Español](README.es.md) | [Português](README.pt-br.md) | **[Русский](README.ru.md)**

[![Go Reference](https://pkg.go.dev/badge/github.com/tron-energy-market/tronzap-sdk-go.svg)](https://pkg.go.dev/github.com/tron-energy-market/tronzap-sdk-go)
[![Go Report Card](https://goreportcard.com/badge/github.com/tron-energy-market/tronzap-sdk-go)](https://goreportcard.com/report/github.com/tron-energy-market/tronzap-sdk-go)
[![CI](https://github.com/tron-energy-market/tronzap-sdk-go/actions/workflows/ci.yml/badge.svg)](https://github.com/tron-energy-market/tronzap-sdk-go/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

Официальный Go SDK для API TronZap.
Данный SDK позволяет легко интегрировать сервисы TronZap для аренды энергии TRON.

TronZap.com позволяет [покупать энергию TRON](https://tronzap.com/), существенно снижая комиссии при переводах USDT (TRC20).

👉 [Зарегистрируйтесь для получения API ключа](https://tronzap.com), чтобы начать использовать TronZap API.

- Сайт: https://tronzap.com
- Справочник API: https://docs.tronzap.com/
- Документация пакета: https://pkg.go.dev/github.com/tron-energy-market/tronzap-sdk-go

## Установка

```bash
go get github.com/tron-energy-market/tronzap-sdk-go
```

## Требования

- Go 1.21 или новее
- Без внешних зависимостей

## Быстрый старт

```go
package main

import (
	"context"
	"fmt"
	"log"

	tronzap "github.com/tron-energy-market/tronzap-sdk-go"
)

func main() {
	client := tronzap.NewClient("ваш_api_token", "ваш_api_secret")
	ctx := context.Background()

	balance, err := client.GetBalance(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("баланс: %s (пополнять на %s)\n", balance.Balance, balance.Address)

	// Рассчитать, сколько энергии нужно для перевода USDT, и купить ровно столько.
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
	fmt.Printf("транзакция %s стоит %s, статус %s\n", tx.ID, tx.Amount, tx.Status)
}
```

Запускаемый разбор всех операций находится в
[`examples/basic`](examples/basic/main.go):

```bash
export TRONZAP_API_TOKEN=ваш_api_token
export TRONZAP_API_SECRET=ваш_api_secret
export TRONZAP_BASE_URL=api.tronzap.com   # опционально
go run ./examples/basic
```

По умолчанию он только читает и ничего не расходует. Если задать
`TRONZAP_ALLOW_PURCHASES=1`, дополнительно вызываются endpoint'ы, создающие
транзакции и AML-проверки, — они списывают средства со счёта. Остальные
опциональные переменные описаны в комментарии в начале файла.

## Настройка

`NewClient` принимает две учётные данные из вашей панели: API-токен отправляется
как bearer-токен, а секрет подписывает тело каждого запроса. Всё остальное —
опции:

```go
client := tronzap.NewClient(apiToken, apiSecret,
	tronzap.WithBaseURL("api.tronzap.com"), // по умолчанию tronzap.DefaultBaseURL
	tronzap.WithTimeout(10*time.Second),    // по умолчанию tronzap.DefaultTimeout (30s)
	tronzap.WithHTTPClient(myClient),       // свой транспорт, прокси или ретраи
	tronzap.WithUserAgent("my-app/1.0"),
)
```

`WithBaseURL` принимает как просто домен, так и полный URL: при отсутствии схемы
подставляется `https`, а завершающий слэш убирается, поэтому
`"api.tronzap.com"`, `"api.tronzap.com/"` и `"https://api.tronzap.com"`
равнозначны. Укажите схему явно, чтобы этого не происходило, например
`"http://localhost:8080"` для локального мока.

`Client` безопасен для одновременного использования из нескольких горутин и не
хранит глобального состояния, поэтому создавайте один клиент на набор учётных
данных и переиспользуйте его. Клиент, переданный через `WithHTTPClient`, никогда
не изменяется: `WithTimeout` применяет таймаут к копии.

Все методы, выполняющие запросы, принимают `context.Context` первым аргументом,
поэтому дедлайны и отмена работают как обычно:

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

services, err := client.GetServices(ctx)
```

## Доступные методы

| Метод | Endpoint | Описание |
|---|---|---|
| `GetServices(ctx)` | `/v1/services` | Доступные сервисы и цены |
| `GetBalance(ctx)` | `/v1/balance` | Текущий баланс аккаунта |
| `GetAddressInfo(ctx, address)` | `/v1/address-info` | Ресурсы адреса (energy, bandwidth) и балансы (TRX, USDT) |
| `EstimateEnergy(ctx, req)` | `/v1/estimate-energy` | Сколько энергии нужно для перевода и её стоимость |
| `Calculate(ctx, req)` | `/v1/calculate` | Расчёт стоимости покупки без создания транзакции |
| `CreateEnergyTransaction(ctx, req)` | `/v1/transaction/new` | Покупка энергии |
| `CreateBandwidthTransaction(ctx, req)` | `/v1/transaction/new` | Покупка bandwidth |
| `CreateResourceBundleTransaction(ctx, req)` | `/v1/transaction/new` | Покупка energy и bandwidth одной транзакцией |
| `CreateAddressActivationTransaction(ctx, req)` | `/v1/transaction/new` | Активация адреса TRON |
| `CheckTransaction(ctx, req)` | `/v1/transaction/check` | Статус транзакции по id или внешнему id |
| `GetDirectRechargeInfo(ctx)` | `/v1/direct-recharge-info` | Адрес и тарифы прямого пополнения |
| `GetAMLServices(ctx)` | `/v1/aml-checks` | AML-сервисы и цены |
| `CreateAMLCheck(ctx, req)` | `/v1/aml-checks/new` | Создание AML-проверки |
| `CheckAMLStatus(ctx, id)` | `/v1/aml-checks/check` | Статус и результат AML-проверки |
| `GetAMLHistory(ctx, req)` | `/v1/aml-checks/history` | История AML-проверок с пагинацией |
| `GetSubscriptions(ctx)` | `/v1/subscriptions` | Планы подписок и цены |
| `StartSubscription(ctx, req)` | `/v1/subscription/start` | Подписать адрес на план |
| `CheckSubscription(ctx, req)` | `/v1/subscription/check` | Статус подписки по id или внешнему id |
| `StopSubscription(ctx, req)` | `/v1/subscription/stop` | Остановить подписку |
| `GetSubscriptionHistory(ctx, req)` | `/v1/subscriptions/history` | История подписок с пагинацией |
| `Do(ctx, endpoint, params, result)` | любой | Прямой доступ к ещё не обёрнутым endpoint'ам |

Опциональные поля вынесены в структуры запросов, а не в длинные списки
параметров, поэтому новые поля API можно добавлять, не ломая ваш код. Нулевые
значения означают «использовать значение API по умолчанию»: `Duration`
становится 1 час, а пагинация истории AML и подписок — страница 1 по 10
элементов. Исключение — `StartSubscriptionRequest`: нулевые `DurationDays` и
`TransactionsLimit` означают отсутствие ограничения.

### Покупка ресурсов

```go
// Энергия, с опциональной активацией адреса в том же вызове.
tx, err := client.CreateEnergyTransaction(ctx, tronzap.EnergyTransactionRequest{
	Address:         "TRecipientAddress",
	Energy:          65000,
	Duration:        1,      // часы; поддерживается только 1
	ExternalID:      "order-42",
	ActivateAddress: true,
})

// Bandwidth.
tx, err = client.CreateBandwidthTransaction(ctx, tronzap.BandwidthTransactionRequest{
	Address:    "TRecipientAddress",
	Bandwidth:  345,
	ExternalID: "bandwidth-1",
})

// Energy и bandwidth вместе — дешевле, чем покупать по отдельности.
tx, err = client.CreateResourceBundleTransaction(ctx, tronzap.ResourceBundleTransactionRequest{
	Address:    "TRecipientAddress",
	Energy:     65000,
	Bandwidth:  345,
	Duration:   1,
	ExternalID: "bundle-1",
})

// Только активация.
tx, err = client.CreateAddressActivationTransaction(ctx, tronzap.AddressActivationRequest{
	Address:    "TRecipientAddress",
	ExternalID: "activation-1",
})
```

### Отслеживание транзакции

Транзакция проходит путь `new` → `pending` → `success` или `failed`:

```go
for {
	tx, err := client.CheckTransaction(ctx, tronzap.CheckTransactionRequest{ExternalID: "order-42"})
	if err != nil {
		return err
	}
	if tx.Status == tronzap.TransactionStatusSuccess || tx.Status == tronzap.TransactionStatusFailed {
		fmt.Println("завершена со статусом", tx.Status, "hash", tx.Hash)
		return nil
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(2 * time.Second):
	}
}
```

### AML-проверка

```go
check, err := client.CreateAMLCheck(ctx, tronzap.AMLCheckRequest{
	Type:    tronzap.AMLTypeAddress, // или tronzap.AMLTypeHash
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

Для проверки по hash `Address` — это адрес получателя средств в транзакции, а
`Direction` указывает, на какой стороне транзакции вы: `AMLDirectionDeposit`,
если средства пришли на ваш адрес (`Address` — ваш адрес),
`AMLDirectionWithdrawal`, если их отправили вы (`Address` — адрес внешнего
получателя). Риск оценивается для контрагента: для deposit — для отправителя,
для withdrawal — для получателя. Если `Direction` пустой, SDK отправляет
`AMLDirectionDeposit`.

`RiskScore` — это `*Number`, потому что API оставляет его null до завершения
проверки; перед чтением проверяйте на nil.

### Подписки

Подписка обеспечивает адрес энергией для каждой транзакции, пока её не
остановят или не закончатся её дни или транзакции. Выберите план из
`GetSubscriptions` и передайте его `SubscriptionID`, например
`"unlimited_energy"`, а не числовой `ID`:

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
	DurationDays:      30, // 0 — без ограничения по времени
	TransactionsLimit: 0,  // 0 — без ограничения
	ExternalID:        "subscription-42",
})

sub, err = client.CheckSubscription(ctx, tronzap.SubscriptionRequest{ExternalID: "subscription-42"})

sub, err = client.StopSubscription(ctx, tronzap.SubscriptionRequest{ID: sub.ID})

history, err := client.GetSubscriptionHistory(ctx, tronzap.SubscriptionHistoryRequest{
	Status: tronzap.SubscriptionStatusActive,
})
```

Запуск, проверка и остановка возвращают подписку с её `Params`, а история
вместо них — счётчики использования `TransactionsUsed`, `EnergyUsed` и
`TotalPrice`. Подписку с лимитом транзакций остановить нельзя
(`CodeCannotStopSubscription`).

## Обработка ошибок

Любой сбой — это обычная ошибка Go. Детали несут четыре конкретных типа, и каждый
из них сопоставляется с сентинелами пакета через `errors.Is`, так что ветвиться
можно настолько точно, насколько нужно:

| Тип | Значение | Совпадает с |
|---|---|---|
| `*APIError` | API ответил с `code`, отличным от нуля | `ErrAPI` |
| `*HTTPError` | Ответ не 2xx без пригодного тела от API | `ErrHTTP`, а также `ErrRateLimit` (429), `ErrUnauthorized` (401/403) или `ErrServer` (5xx) |
| `*NetworkError` | Ответ не пришёл вообще | `ErrNetwork`, а также `ErrConnection`, `ErrTimeout` или `ErrTLS` |
| `*InvalidResponseError` | Ответ 2xx, который SDK не смог разобрать | `ErrInvalidResponse` |

Аргументы, которые SDK отклоняет до отправки запроса, совпадают с
`ErrInvalidRequest`.

```go
var apiErr *tronzap.APIError
switch {
case errors.As(err, &apiErr):
	// Ошибка уровня приложения: код точно говорит, что произошло.
	switch apiErr.Code {
	case tronzap.CodeInvalidTronAddress:
		// apiErr.Key может уточнить, например "invalid_tron_address.from_address"
		log.Println("неверный адрес:", apiErr.Key)
	case tronzap.CodeInsufficientFunds:
		log.Println("пополните баланс")
	case tronzap.CodeAddressNotActivated:
		log.Println("сначала активируйте адрес")
	default:
		log.Printf("ошибка api %d: %s (запрос %s)", apiErr.Code, apiErr.Message, apiErr.RequestID)
	}
case errors.Is(err, tronzap.ErrRateLimit):
	// Подождать и повторить.
case errors.Is(err, tronzap.ErrUnauthorized):
	// Неверный токен или подпись.
case errors.Is(err, tronzap.ErrTimeout), errors.Is(err, tronzap.ErrServer):
	// Временный сбой; повтор безопасен.
case errors.Is(err, tronzap.ErrNetwork):
	// Недоступно.
}
```

`APIError.RequestID` — идентификатор, который API присваивает каждому запросу;
указывайте его при обращении в поддержку.

Поскольку `NetworkError` оборачивает исходную транспортную ошибку, сбои контекста
остаются напрямую различимыми:

```go
if errors.Is(err, context.Canceled) { /* вызывающий отменил запрос */ }
if errors.Is(err, context.DeadlineExceeded) { /* истёк дедлайн */ }
```

Ошибка API приоритетнее HTTP-статуса: часть сбоев API возвращает со статусом 2xx,
а часть — с 4xx или 5xx, поэтому разбираемое тело с кодом, отличным от нуля,
всегда возвращается как `*APIError`, а не как `*HTTPError`.

### Коды ошибок API

| Код | Константа | Описание |
|-----|-----------|----------|
| 1 | `CodeAuth` | Ошибка аутентификации — неверный API-токен или подпись |
| 2 | `CodeInvalidServiceOrParams` | Некорректный сервис или параметры |
| 5 | `CodeWalletNotFound` | Внутренний кошелёк не найден. Обратитесь в поддержку. |
| 6 | `CodeInsufficientFunds` | Недостаточно средств |
| 10 | `CodeInvalidTronAddress` | Некорректный адрес TRON, или у адреса уже есть активная подписка |
| 11 | `CodeInvalidEnergyAmount` | Некорректное количество энергии |
| 12 | `CodeInvalidDuration` | Некорректная длительность |
| 20 | `CodeTransactionNotFound` | Транзакция/подписка не найдена |
| 21 | `CodeCannotStopSubscription` | Невозможно остановить подписку, например, у неё есть лимит транзакций |
| 24 | `CodeAddressNotActivated` | Адрес не активирован |
| 25 | `CodeAddressAlreadyActivated` | Адрес уже активирован |
| 30 | `CodeAMLCheckNotFound` | AML-проверка не найдена |
| 35 | `CodeServiceNotAvailable` | Сервис недоступен |
| 50 | `CodeInvalidBandwidthAmount` | Некорректное количество bandwidth |
| 500 | `CodeInternalServerError` | Внутренняя ошибка сервера — обратитесь в поддержку |

## Числовые поля и даты

В одних ответах API отдаёт денежные значения числом JSON, в других — строкой,
поэтому суммы и цены используют тип `Number`, который разбирает оба варианта и
предоставляет `Float64()` и `String()`. Для дат используется `Time`: он встраивает
`time.Time`, принимает все форматы, которые отдаёт API, и сохраняет исходный текст
в `Raw`. Нераспознанная дата оставляет встроенное время нулевым, вместо того чтобы
завалить разбор всего ответа.

## Доступ к необёрнутым endpoint'ам

`Do` подписывает и отправляет запрос на любой endpoint и разбирает поле `result` в
переданное вами значение — так можно использовать возможности API, которые SDK
пока не обёрнул:

```go
var result struct {
	Items []struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	} `json:"items"`
}
err := client.Do(ctx, "/v1/new-endpoint", map[string]any{"page": 1}, &result)
```

## Тестирование

```bash
go test ./...
go test -race -cover ./...
go vet ./...
gofmt -l .
```

## Лицензия

Лицензия MIT (MIT). Подробности в [файле лицензии](LICENSE).

## Поддержка

По вопросам поддержки пишите на [support@tronzap.com](mailto:support@tronzap.com).
