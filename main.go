package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/adshao/go-binance"
	"github.com/adshao/go-binance/futures"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
)

func main() {
	chatid, tgkey, key, skey, riskstr := readSettings()
	chatid = strings.TrimSuffix(chatid, "\r")
	key = strings.TrimSuffix(key, "\r")
	skey = strings.TrimSuffix(skey, "\r")
	riskstr = strings.TrimSuffix(riskstr, "\r")
	risk, _ := strconv.ParseFloat(riskstr, 64)
	if chatid == "" {
		log.Println("Нет chat ID пользователя в ТГ, в файле Options.txt")
		exit()
	}
	if key == "" {
		log.Println("Нет ключа api в файле Options.txt")
		exit()
	}
	if skey == "" {
		log.Println("Нет секретного ключа в файле Options.txt")
		exit()
	}
	if tgkey == "" {
		log.Println("Нет ключа бота в ТГ в файле Options.txt")
		exit()
	}
	bot, err := tgbotapi.NewBotAPI(strings.TrimSuffix(tgkey, "\r"))
	if err != nil {
		log.Panic(err)
	}
	bot.Debug = true

	log.Printf("Authorized on account %s", bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates, err := bot.GetUpdatesChan(u)
	go ordersAlerts(bot, chatid, key, skey)
	go balanceAlerts(bot, chatid, key, skey)
	for update := range updates {
		if update.Message == nil { // ignore any non-Message Updates
			continue
		}

		log.Printf("[%s] %s", update.Message.From.UserName, update.Message.Text)

		msg := tgbotapi.NewMessage(update.Message.Chat.ID, update.Message.Text)
		if strconv.FormatInt(update.Message.Chat.ID, 10) == chatid {
			msg.ReplyToMessageID = update.Message.MessageID
			data := strings.Split(msg.Text, " ")

			switch {
			case data[0] == "help" || data[0] == "Help":
				msg.Text = "Об исполнение ордеров бот уведомляет автоматически.\nКоманды:\norder символ без usdt первый ордер последний ордер стоп.\nПример: order BTC 23000 23500 23700.\nlist список активных ордеров.\nrisk установка риска на позицию в процентах.\nПример: risk 1."
				bot.Send(msg)
			case data[0] == "order" || data[0] == "Order":
				// data[0] - command, data[1] - symbol, data[2] - start price, data[3] - orders stop price, data[4] - stop price
				symbol := data[1] + "USDT"
				startOrdersPrice, err := strconv.ParseFloat(data[2], 64)
				if err != nil {
					fmt.Println(err)
					msg.Text = err.Error()
					bot.Send(msg)
					continue
				}
				stopOrdersPrice, err := strconv.ParseFloat(data[3], 64)
				if err != nil {
					fmt.Println(err)
					msg.Text = err.Error()
					bot.Send(msg)
					continue
				}
				stopPrice, err := strconv.ParseFloat(data[4], 64)
				if err != nil {
					fmt.Println(err)
					msg.Text = err.Error()
					bot.Send(msg)
					continue
				}
				qty, pricePrecison, qtyPrecison := calculatePositionQty(symbol, risk, startOrdersPrice, stopOrdersPrice, stopPrice, key, skey)
				priceStep := math.Abs((startOrdersPrice - stopOrdersPrice) / 10)
				qtyStep := round(qty/10, qtyPrecison)
				log.Printf("priceStep %#v", priceStep)
				qty = 0
				avg := 0.0
				for i := 0; i < 11; i++ {
					if startOrdersPrice < stopPrice { //short
						createOrders(symbol, "Short", fmt.Sprintf("%f", startOrdersPrice), fmt.Sprintf("%f", qtyStep), key, skey)
						startOrdersPrice = round(startOrdersPrice+priceStep, pricePrecison)
					} else { //long
						createOrders(symbol, "Long", fmt.Sprintf("%f", startOrdersPrice), fmt.Sprintf("%f", qtyStep), key, skey)
						startOrdersPrice = round(startOrdersPrice-priceStep, pricePrecison)
					}
					qty = qty + qtyStep
					avg = avg + startOrdersPrice
				}
				avg = avg / 11
				if startOrdersPrice < stopPrice { //short
					createStopOrder(symbol, "Long", fmt.Sprintf("%f", stopPrice), fmt.Sprintf("%f", qty), key, skey)
				} else { //long
					createStopOrder(symbol, "Short", fmt.Sprintf("%f", stopPrice), fmt.Sprintf("%f", qty), key, skey)
				}
				if qty == 0 {
					msg.Text = "Сумма ордера < 0. Установите стоп-лосс ближе или измените параметры риска."
					bot.Send(msg)
				} else {
					msg.Text = "Ордера созданы. Средняя цена: " + fmt.Sprintf("%f", avg) + ". Кол-во: " + fmt.Sprintf("%f", qty)
					bot.Send(msg)
				}

			case data[0] == "list" || data[0] == "List":
				msg.Text = "Ищу открытые ордера"
				bot.Send(msg)
				msg.Text = getOrders(key, skey)
				bot.Send(msg)
			case data[0] == "Risk" || data[0] == "risk":
				// data[0] - command, data[1] - risk
				risk, _ = strconv.ParseFloat(data[1], 64)
				risk = risk / 100
				riskReplace(fmt.Sprintf("%f", risk))
				msg.Text = "Риск установлен."
				bot.Send(msg)
			case data[0] == "Spot" || data[0] == "spot":
				client := binance.NewClient(key, skey)
				client.NewSetServerTimeService().Do(context.Background())
				account, err := client.NewGetAccountService().Do(context.Background())
				if err != nil {
					fmt.Println(err)
					return
				}
				for _, o := range account.Balances {
					balance, _ := strconv.ParseFloat(o.Free, 64)
					if balance != 0 {
						if o.Asset != "BNB" {
							msg.Text = msg.Text + ListOrders(client, o.Asset, "BTC") + ListOrders(client, o.Asset, "USDT") + ListOrders(client, o.Asset, "BUSD") + "\n"
						}
					}
				}
				bot.Send(msg)
			case data[0] == "Balance" || data[0] == "balance":

				balanceS, balanceF := GetBalance(key, skey)
				msg.Text = fmt.Sprintf("Balance Sum: %f$\nBalance Futures: %f$\nBalance Spot: %f$", balanceS+balanceF, balanceF, balanceS)
				bot.Send(msg)
			case data[0] == "Sell" || data[0] == "sell":
				//sell symbol price qty
				fmt.Printf(data[0] + " " + data[1] + " " + data[2] + " " + data[3])
				if len(data) < 4 {
					msg.Text = "Неверная команда. Пример sell BTCUSDT 60000 0.1"
					bot.Send(msg)
				}
				go Sell(bot, chatid, key, skey, data[1], data[2], data[3])
			default:
				msg.Text = "Нет такой функции."
				bot.Send(msg)
			}
		}
	}
}

func exit() {
	fmt.Println("Press 'q' to quit")
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		exit := scanner.Text()
		if exit == "q" {
			os.Exit(2)
		} else {
			fmt.Println("Press 'q' to quit")
		}
	}
}

func Sell(bot *tgbotapi.BotAPI, chatid string, key string, skey string, symbol string, price string, qty string) {
	client := binance.NewClient(key, skey)
	client.NewSetServerTimeService().Do(context.Background())
	chat, _ := strconv.ParseInt(chatid, 10, 64)
	msg := tgbotapi.NewMessage(chat, "")
	orderPrice, _ := strconv.ParseFloat(price, 64)
	klines, err := client.NewKlinesService().Symbol(strings.ToUpper(symbol)).
		Interval("1m").Do(context.Background())
	if err != nil {
		fmt.Println(err)
	}
	currentPrice, _ := strconv.ParseFloat(klines[len(klines)-1].Close, 64)
	if currentPrice > orderPrice {
		msg.Text = "Цена ордера ниже текущей цены."
		bot.Send(msg)
		return
	} else {
		for {

			if (currentPrice/orderPrice - 1) < 0.7 {
				_, err := client.NewCreateOrderService().Symbol(strings.ToUpper(symbol)).
					Side(binance.SideTypeSell).Type(binance.OrderTypeLimit).
					TimeInForce(binance.TimeInForceTypeGTC).Quantity(qty).
					Price(price).Do(context.Background())
				if err != nil {
					msg.Text = err.Error()
					bot.Send(msg)
					return
				}
				msg.Text = "Ордер выставлен: " + symbol + " " + strings.ToUpper(symbol) + " " + qty
				bot.Send(msg)
				return
			} else {
				time.Sleep(time.Minute)
				klines, _ = client.NewKlinesService().Symbol(strings.ToUpper(symbol)).
					Interval("1m").Do(context.Background())
				currentPrice, _ = strconv.ParseFloat(klines[len(klines)-1].Close, 64)
			}
		}
	}
}

func GetBalance(key string, skey string) (spotBalance float64, futuresBalance float64) {
	client := binance.NewClient(key, skey)
	client.NewSetServerTimeService().Do(context.Background())
	account, err := client.NewGetAccountService().Do(context.Background())
	if err != nil {
		fmt.Println(err)
	}
	futuresClient := binance.NewFuturesClient(key, skey)
	futuresClient.NewSetServerTimeService().Do(context.Background())
	accountFutures, err := futuresClient.NewGetBalanceService().Do(context.Background())
	if err != nil {
		fmt.Println(err)
	}
	futuresBalance, _ = strconv.ParseFloat(accountFutures[0].Balance, 64)
	for _, o := range account.Balances {
		balance, _ := strconv.ParseFloat(o.Free, 64)
		balanceLocked, _ := strconv.ParseFloat(o.Locked, 64)
		if balance != 0 || balanceLocked != 0 {
			if o.Asset == "USDT" {
				spotBalance = spotBalance + balance + balanceLocked
			} else {
				klines, err := client.NewKlinesService().Symbol(o.Asset + "USDT").
					Interval("1m").Do(context.Background())
				if err != nil {
					fmt.Println(err)
					klines, err = client.NewKlinesService().Symbol(o.Asset + "BUSD").
						Interval("1m").Do(context.Background())
					if err != nil {
						fmt.Println(err)
						continue
					}
				}
				currentPrice, _ := strconv.ParseFloat(klines[len(klines)-1].Close, 64)
				spotBalance = spotBalance + balance*currentPrice + balanceLocked*currentPrice
			}
		}
	}

	return spotBalance, futuresBalance
}

func balanceAlerts(bot *tgbotapi.BotAPI, chatid string, key string, skey string) {
	var balanceOld, balance float64
	check := false
	chat, _ := strconv.ParseInt(chatid, 10, 64)
	client := binance.NewClient(key, skey)
	msg := tgbotapi.NewMessage(chat, "")
	for {
		fmt.Println("Checking balance")
		client.NewSetServerTimeService().Do(context.Background())
		if check == false {
			balanceS, balanceF := GetBalance(key, skey)
			balanceOld = balanceS + balanceF
			check = true
		} else {
			balanceS, balanceF := GetBalance(key, skey)
			balance = balanceS + balanceF
			percent := balanceOld * 0.1
			switch {
			case balance-balanceOld > percent:
				change := (balance/balanceOld - 1) * 100
				msg.Text = "Хоп хей ла ла лей, баланс вырос на " + fmt.Sprintf("%0.1f", change) + ".\n Текущий баланс: " + fmt.Sprintf("%f", balance)
				balanceOld = balance
				bot.Send(msg)
			case balance-balanceOld < -percent:
				change := (balanceOld/balance - 1) * 100
				msg.Text = "ПечальБеда, баланс упал на " + fmt.Sprintf("%0.1f", change) + ".\n Текущий баланс: " + fmt.Sprintf("%f", balance)
				balanceOld = balance
				msg = tgbotapi.NewMessage(chat, msg.Text)
				bot.Send(msg)
			}
			fmt.Println("Current old Balance", balanceOld)
			fmt.Println("Current change", balance-balanceOld, "percent", percent)
		}
		time.Sleep(time.Minute * 10)
	}
}

func ordersAlerts(bot *tgbotapi.BotAPI, chatid string, key string, skey string) {
	var id []int64
	var symbol []string
	chat, _ := strconv.ParseInt(chatid, 10, 64)
	msg := tgbotapi.NewMessage(chat, "")
	futuresClient := binance.NewFuturesClient(key, skey)
	futuresClient.NewSetServerTimeService().Do(context.Background())
	exchange, _ := futuresClient.NewExchangeInfoService().Do(context.Background())
	for {
		for _, s := range exchange.Symbols {
			openOrders, err := futuresClient.NewListOpenOrdersService().Symbol(s.Symbol).Do(context.Background())
			if err != nil {
				fmt.Println(err)
				break
			}
			for _, o := range openOrders {
				if !inArray(o.OrderID, id) {
					id = append(id, o.OrderID)
					symbol = append(symbol, o.Symbol)
				}
			}
		}
		for i := 0; i < len(id); i++ {
			order, err := futuresClient.NewGetOrderService().Symbol(symbol[i]).OrderID(id[i]).Do(context.Background())
			if err != nil {
				fmt.Println(err)
				break
			}
			if order.Status == futures.OrderStatusTypeFilled {
				symbol[i] = ""
				id[i] = 0
				log.Println("Ордер исполнен: " + order.Symbol + " " + string(order.Side) + " Цена:" + order.Price + " Кол-во:" + order.OrigQuantity)
				msg.Text = "Ордер исполнен: " + order.Symbol + " " + string(order.Side) + " Цена:" + order.Price + " Кол-во:" + order.OrigQuantity
				bot.Send(msg)
			}
			if order.Status == futures.OrderStatusTypeCanceled {
				symbol[i] = ""
				id[i] = 0
			}
		}
		id, symbol = delete(id, symbol)
		log.Println(id)
	}
}

func getOrders(key string, skey string) (str string) {
	futuresClient := binance.NewFuturesClient(key, skey)
	futuresClient.NewSetServerTimeService().Do(context.Background())
	exchange, _ := futuresClient.NewExchangeInfoService().Do(context.Background())
	for _, s := range exchange.Symbols {
		openOrders, err := futuresClient.NewListOpenOrdersService().Symbol(s.Symbol).Do(context.Background())
		if err != nil {
			fmt.Println(err)
			return
		}
		for _, o := range openOrders {
			str = str + o.Symbol + " " + string(o.Side) + " " + o.Price + " Кол-во: " + o.OrigQuantity + "\n"
		}
	}
	log.Println(str)
	return str
}

func riskReplace(risk string) {

	data := []byte(risk)
	file, err := os.Create("Risk.txt")
	if err != nil {
		fmt.Println("Unable to create file:", err)
	}
	defer file.Close()
	file.Write(data)

}

func readSettings() (chatid string, tgkey string, key string, skey string, risk string) {
	var split []string
	var keys string
	file, err := os.Open("Options.txt")
	defer file.Close()
	if err != nil {
		log.Println("Файл Options.txt не найден. Создан новый. Укажите:\nChat ID в ТГ\nКлюч для бота в ТГ \nКлюч Api Binance \nСекретный ключ Api Binance ")
		os.Create("Options.txt")
		exit()
		return "", "", "", "", ""
	}

	data := make([]byte, 64)

	for {
		n, err := file.Read(data)
		if err == io.EOF { // если конец файла
			break // выходим из цикла
		}
		keys = keys + string(data[:n])
	}
	log.Print(keys)
	split = strings.Split(keys, "\n")
	if len(split) < 4 {
		log.Println("Файл Options.txt заполнен не корректно. Укажите:\nНик в ТГ\nКлюч для бота в ТГ \nКлюч Api Binance \nСекретный ключ Api Binance ")
		exit()
	}
	file.Close()
	if _, err := os.Stat("Risk.txt"); os.IsNotExist(err) {
		os.Create("Risk.txt")
		risk = "0.01"
	} else {
		file, err = os.Open("Risk.txt")
		data = make([]byte, 64)

		for {
			n, err := file.Read(data)
			if err == io.EOF { // если конец файла
				break // выходим из цикла
			}
			risk = risk + string(data[:n])
		}
	}
	return split[0], split[1], split[2], split[3], risk
}

func createStopOrder(symbol string, side string, price string, qty string, key string, skey string) {
	futuresClient := binance.NewFuturesClient(key, skey)
	futuresClient.NewSetServerTimeService().Do(context.Background())
	var sideType futures.SideType
	switch side {
	case "Short":
		sideType = futures.SideTypeSell
	case "Long":
		sideType = futures.SideTypeBuy
	}
	order, err := futuresClient.NewCreateOrderService().Symbol(symbol).
		Side(sideType).Type(futures.OrderTypeStopMarket).
		TimeInForce(futures.TimeInForceTypeGTC).Quantity(qty).
		StopPrice(price).ReduceOnly(true).Do(context.Background())
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(order)
}

func createOrders(symbol string, side string, price string, qty string, key string, skey string) {
	futuresClient := binance.NewFuturesClient(key, skey)
	futuresClient.NewSetServerTimeService().Do(context.Background())
	var sideType futures.SideType
	switch side {
	case "Short":
		sideType = futures.SideTypeSell
	case "Long":
		sideType = futures.SideTypeBuy
	}
	order, err := futuresClient.NewCreateOrderService().Symbol(symbol).
		Side(sideType).Type(futures.OrderTypeLimit).
		TimeInForce(futures.TimeInForceTypeGTX).Quantity(qty).
		Price(price).Do(context.Background())
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(order)
}

func calculatePositionQty(symbol string, risk float64, startOrdersPrice float64, stopOrdersPrice float64, stopPrice float64, key string, skey string) (qty float64, pricePrecison int, qtyPrecison int) {
	var stop float64
	futuresClient := binance.NewFuturesClient(key, skey)
	futuresClient.NewSetServerTimeService().Do(context.Background())
	res, err := futuresClient.NewGetAccountService().Do(context.Background())
	if err != nil {
		fmt.Println(err)
		return
	}
	exchange, _ := futuresClient.NewExchangeInfoService().Do(context.Background())
	entryPrice := (startOrdersPrice + stopOrdersPrice) / 2
	log.Printf("entryPrice %#v", entryPrice)
	if entryPrice < stopPrice { //short
		stop = stopPrice/entryPrice - 1 //stopLoss in percents
		log.Printf("stop %#v", stop)
	} else { //long
		stop = entryPrice/stopPrice - 1
		log.Printf("stop %#v", stop)
	}
	balance, _ := strconv.ParseFloat(res.TotalWalletBalance, 64)
	log.Printf("balance %#v", balance)
	positionSize := balance * risk / stop
	log.Printf("positionSize %#v", positionSize)
	prices, err := futuresClient.NewListPricesService().Do(context.Background())
	if err != nil {
		fmt.Println(err)
		return
	}
	for _, p := range prices {
		if p.Symbol == symbol {
			for _, s := range exchange.Symbols {
				if s.Symbol == symbol {
					pricePrecison = s.PricePrecision
					qtyPrecison = s.QuantityPrecision
				}
			}
			price, _ := strconv.ParseFloat(p.Price, 64)
			qty = positionSize / round(price, pricePrecison)
			break
		}
	}
	fmt.Println("qty: ", qty)
	fmt.Println("pricePrecison: ", pricePrecison)
	fmt.Println("qtyPrecison: ", qtyPrecison)
	return round(qty, qtyPrecison), pricePrecison, qtyPrecison
}

func round(x float64, prec int) float64 {
	var rounder float64
	pow := math.Pow(10, float64(prec))
	intermed := x * pow
	_, frac := math.Modf(intermed)
	if frac >= 0.5 {
		rounder = math.Ceil(intermed)
	} else {
		rounder = math.Floor(intermed)
	}

	return rounder / pow
}

func delete(arrayInt []int64, arrayString []string) (ari []int64, ars []string) {

	for i := 0; i < len(arrayInt); i++ {
		if arrayInt[i] == 0 {
			arrayString[i] = arrayString[len(arrayString)-1]
			arrayString[len(arrayString)-1] = ""
			arrayInt[i] = arrayInt[len(arrayInt)-1]
			arrayInt[len(arrayInt)-1] = 0
			arrayString = arrayString[:len(arrayString)-1]
			arrayInt = arrayInt[:len(arrayInt)-1]
		}
	}
	return arrayInt, arrayString
}

func inArray(val interface{}, array interface{}) (exists bool) {
	exists = false

	switch reflect.TypeOf(array).Kind() {
	case reflect.Slice:
		s := reflect.ValueOf(array)

		for i := 0; i < s.Len(); i++ {
			if reflect.DeepEqual(val, s.Index(i).Interface()) == true {
				exists = true
				return
			}
		}
	}

	return
}

func ListOrders(client *binance.Client, symbol string, ticker string) (data string) {
	var cumQty, posPrice, orderPrice, currentPrice float64
	var first bool
	orders, err := client.NewListOrdersService().Symbol(symbol + ticker).
		Do(context.Background())
	if err != nil {
		return
	}
	for _, o := range orders {
		//market order price = cummulativeQuoteQty/executedQty
		if o.Status == binance.OrderStatusTypeFilled {
			if o.Side == binance.SideTypeBuy {
				qty, _ := strconv.ParseFloat(o.OrigQuantity, 64)
				if first {
					if o.Type == binance.OrderTypeLimit {
						posPrice, err = strconv.ParseFloat(o.Price, 64)
						if err != nil {
							fmt.Println(err)
						}
					} else {
						quote, err := strconv.ParseFloat(o.CummulativeQuoteQuantity, 64)
						if err != nil {
							fmt.Println(err)
						}
						posPrice = quote / qty
					}
				} else {
					if o.Type == binance.OrderTypeLimit {
						orderPrice, _ = strconv.ParseFloat(o.Price, 64)
					} else {
						quote, err := strconv.ParseFloat(o.CummulativeQuoteQuantity, 64)
						if err != nil {
							fmt.Println(err)
						}
						orderPrice = quote / qty
					}
					posPrice = posPrice*(cumQty/(cumQty+qty)) + orderPrice*(qty/(cumQty+qty))
				}
				cumQty = cumQty + qty
			} else {
				qty, _ := strconv.ParseFloat(o.OrigQuantity, 64)
				cumQty = cumQty - qty
			}
		}
	}
	switch {
	case posPrice*cumQty > 10 && (ticker == "USDT" || ticker == "BUSD") && symbol != "BUSD":
		klines, err := client.NewKlinesService().Symbol(symbol + ticker).
			Interval("1m").Do(context.Background())
		if err != nil {
			fmt.Println(err)
			return
		}
		currentPrice, _ = strconv.ParseFloat(klines[len(klines)-1].Close, 64)
		pnl := cumQty*currentPrice - cumQty*posPrice
		fmt.Println(symbol, "USDT QTY:", cumQty*posPrice, "QTY:", cumQty, "AvgPrice:", posPrice, "PNL:", pnl)
		data = "\n" + symbol + "/" + ticker + fmt.Sprintf("\nUSDT QTY: %f", cumQty*posPrice) + fmt.Sprintf("\nQTY: %f", cumQty) + fmt.Sprintf("\nAvgPrice: %f", posPrice) + fmt.Sprintf("\nPrice: %f", currentPrice) + fmt.Sprintf("\nPNL: %f", pnl) + "$"
		return data
	case round(cumQty, 8) > 0 && ticker == "BTC":
		klines, err := client.NewKlinesService().Symbol(symbol + ticker).
			Interval("1m").Do(context.Background())
		if err != nil {
			fmt.Println(err)
			return
		}
		klinesBTC, err := client.NewKlinesService().Symbol("BTCUSDT").
			Interval("1m").Do(context.Background())
		if err != nil {
			fmt.Println(err)
			return
		}
		currentPrice, _ = strconv.ParseFloat(klines[len(klines)-1].Close, 64)
		BTCprice, _ := strconv.ParseFloat(klinesBTC[len(klinesBTC)-1].Close, 64)
		pnl := cumQty*currentPrice - cumQty*posPrice
		fmt.Println(symbol, "BTC QTY:", cumQty*posPrice, "QTY:", cumQty, "AvgPrice:", posPrice, "PNL:", pnl, "USDT PNL:", pnl*BTCprice)
		data = "\n" + symbol + "/" + ticker + fmt.Sprintf("\nBTC QTY: %f", cumQty*posPrice) + fmt.Sprintf("\nQTY: %f", cumQty) + fmt.Sprintf("\nAvgPrice: %.8f", posPrice) + fmt.Sprintf("\nPrice: %.8f", currentPrice) + fmt.Sprintf("\nPNL: %.8f", pnl) + " BTC" + fmt.Sprintf("\nUSDT PNL:  %f", pnl*BTCprice) + "$"
		return data
	}
	return
}
