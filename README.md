# Exchanger
### Торговый помощник для маржинальной торговли на Binance.com
Работает только с фьючерсами USDT
  
#### Сказать спасибо
BTC: 1DXBapnm5H8djZBEBd1LKSYpy6hRepBr7u  
ETH ERC20: 0x8a5df2a88a6f7c4116d6ff590a0eb2102cdae5b6  
USDT TRC20: TEWZoBgyLDRHWqKCCEjSkqRW1WnB3WL2hR

#### Imports
- github.com/adshao/go-binance
- github.com/go-telegram-bot-api/telegram-bot-api

#### Как работает
Первый запуск создаст файлы Options.txt и Risk.txt.  
Options.txt содержит:  
```
telegram chat id - используйте @chat_id_echo_bot, для того чтобы узнать свой Chat id
binance api key
binance api secret key
telegram bot api key - получить у @BotFather
```
Бот умеет выполнять следующие команды:  
help - повторный вывод доступных команд.  
order BTC 10000 11000 12000 - Бот создаст 10 ордеров на продажу от 10000 до 11000 и установит стоп на 12000.  
Все пары указывать без приставки USDT.  
list - показывает список открытых ордеров.  
risk - установка процента от баланса, который будет использован для расчета объема сделки, при использовании команды order.  
Бот автоматически уведомляет об исполненных ордерах. В связи с тем, что бот проверяет ордера по всем парам на Binance, ордера, которые были созданы и выполнены в течении 10-15 секунд, могут не попасть в отслеживание.  

Для связи: @BlndMrn

