package main
package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Структуры для цен
type CryptoPrice struct {
	USD float64 `json:"usd"`
}

type BinancePrice struct {
	Price string `json:"price"`
}

type NFTStats struct {
	Symbol      string  `json:"symbol"`
	FloorPrice  int64   `json:"floorPrice"`
	ListedCount int     `json:"listedCount"`
	VolumeAll   float64 `json:"volumeAll"`
}

// Структура для настроек уведомлений
type NotificationSettings struct {
	Enabled  bool
	Interval time.Duration
}

// Глобальные переменные
var notificationSettings = make(map[int64]*NotificationSettings)

// Кэш для цен
var priceCache = struct {
	sync.RWMutex
	prices map[string]struct {
		price float64
		time  time.Time
	}
}{
	prices: make(map[string]struct {
		price float64
		time  time.Time
	}),
}

// Альтернативные API для получения цен
func getPriceFromBinance(symbol string) (float64, error) {
	url := fmt.Sprintf("https://api.binance.com/api/v3/ticker/price?symbol=%sUSDT", symbol)
	
	resp, err := http.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return 0, fmt.Errorf("Binance API недоступно")
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	var result BinancePrice
	err = json.Unmarshal(body, &result)
	if err != nil {
		return 0, err
	}

	price, err := strconv.ParseFloat(result.Price, 64)
	if err != nil {
		return 0, err
	}

	return price, nil
}

func getPriceFromCoinGecko(coin string) (float64, error) {
	// Увеличиваем задержку до 5 секунд
	time.Sleep(5 * time.Second)
	
	url := fmt.Sprintf("https://api.coingecko.com/api/v3/simple/price?ids=%s&vs_currencies=usd", coin)

	resp, err := http.Get(url)
	if err != nil {
		return 0, fmt.Errorf("ошибка запроса: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 429 {
		return 0, fmt.Errorf("превышен лимит запросов к API. Попробуйте позже")
	}
	
	if resp.StatusCode != 200 {
		return 0, fmt.Errorf("API недоступно, статус: %d", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("ошибка чтения ответа: %v", err)
	}

	var result map[string]CryptoPrice
	err = json.Unmarshal(body, &result)
	if err != nil {
		return 0, fmt.Errorf("ошибка парсинга JSON: %v", err)
	}

	if coinData, exists := result[coin]; exists {
		return coinData.USD, nil
	}

	return 0, fmt.Errorf("цена не найдена для %s", coin)
}

// Умная функция получения цены с приоритетом Binance
func getCryptoPrice(coin string) (float64, error) {
	// Сначала пробуем Binance (быстрее и без лимитов)
	var price float64
	var err error

	switch coin {
	case "bitcoin":
		price, err = getPriceFromBinance("BTC")
		if err != nil {
			log.Printf("Binance не доступен для BTC, пробуем CoinGecko")
			price, err = getPriceFromCoinGecko("bitcoin")
		}
	case "zcash":
		price, err = getPriceFromBinance("ZEC")
		if err != nil {
			log.Printf("Binance не доступен для ZEC, пробуем CoinGecko")
			price, err = getPriceFromCoinGecko("zcash")
		}
	default:
		price, err = getPriceFromCoinGecko(coin)
	}

	return price, err
}

// Функция с кэшированием цен
func getCryptoPriceWithCache(coin string) (float64, error) {
	// Проверяем кэш
	priceCache.RLock()
	if cached, exists := priceCache.prices[coin]; exists {
		if time.Since(cached.time) < 5*time.Minute { // Увеличили кэш до 5 минут
			priceCache.RUnlock()
			return cached.price, nil
		}
	}
	priceCache.RUnlock()

	// Получаем свежую цену
	price, err := getCryptoPrice(coin)
	if err != nil {
		return 0, err
	}

	// Обновляем кэш
	priceCache.Lock()
	priceCache.prices[coin] = struct {
		price float64
		time  time.Time
	}{price: price, time: time.Now()}
	priceCache.Unlock()

	return price, nil
}

func getNFTPrice(collectionSymbol string) (*NFTStats, error) {
	// Задержка для NFT API
	time.Sleep(1 * time.Second)
	
	collectionSymbol = strings.TrimSpace(collectionSymbol)
	collectionSymbol = strings.ToLower(collectionSymbol)
	collectionSymbol = strings.ReplaceAll(collectionSymbol, " ", "_")

	url := fmt.Sprintf("https://api-mainnet.magiceden.dev/v2/collections/%s/stats", collectionSymbol)

	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("коллекция не найдена")
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var stats NFTStats
	err = json.Unmarshal(body, &stats)
	if err != nil {
		return nil, err
	}

	return &stats, nil
}

// Функция для уведомлений о ZEC с настраиваемым интервалом
func startZECNotifications(bot *tgbotapi.BotAPI) {
	ticker := time.NewTicker(10 * time.Minute) // Увеличили интервал до 10 минут

	go func() {
		for range ticker.C {
			for chatID, settings := range notificationSettings {
				if !settings.Enabled {
					continue
				}

				price, err := getCryptoPriceWithCache("zcash")
				if err != nil {
					if strings.Contains(err.Error(), "превышен лимит") {
						log.Printf("Лимит API превышен, пропускаем уведомление")
						// Не отправляем сообщение об ошибке пользователю
						continue
					}
					log.Printf("Ошибка получения цены ZEC: %v", err)
					continue
				}

				if price < 0.1 {
					log.Printf("Пропускаем нулевую цену ZEC: $%.2f", price)
					continue
				}

				message := fmt.Sprintf("⏰ ZEC Price Update\n💰 $%.2f\n📊 Интервал: %v",
					price, settings.Interval)

				msg := tgbotapi.NewMessage(chatID, message)
				bot.Send(msg)
			}
		}
	}()
}

func formatCollectionName(symbol string) string {
	name := strings.ReplaceAll(symbol, "_", " ")
	name = strings.Title(name)
	return name
}

// Функция для парсинга интервала
func parseInterval(input string) (time.Duration, error) {
	if minutes, err := strconv.Atoi(input); err == nil {
		return time.Duration(minutes) * time.Minute, nil
	}

	duration, err := time.ParseDuration(input)
	if err != nil {
		return 0, fmt.Errorf("неверный формат интервала. Примеры: 5 (минут), 5m, 1h, 30s")
	}
	return duration, nil
}

func main() {
	// Инициализируем random
	rand.Seed(time.Now().UnixNano())

	// Получаем порт из переменных окружения Render
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	token := getToken()
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Panic(err)
	}

	bot.Debug = true
	log.Printf("✅ Бот %s запущен! 🚀", bot.Self.UserName)

	// Запускаем уведомления ZEC
	startZECNotifications(bot)

	// Запускаем HTTP сервер для порта
	go func() {
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, "Bot is running!")
		})
		log.Printf("🌐 Server listening on port %s", port)
		http.ListenAndServe(":"+port, nil)
	}()

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		if update.Message == nil {
			continue
		}

		var msgText string
		text := strings.TrimSpace(update.Message.Text)
		chatID := update.Message.Chat.ID

		switch {
		case text == "/start":
			msgText = "👋 Crypto & NFT Tracker Bot\n\n" +
				"💰 Криптовалюты:\n" +
				"/btc - цена Bitcoin\n" +
				"/zec - цена Zcash\n" +
				"/notify_zec - уведомления ZEC\n" +
				"/interval <время> - изменить интервал\n" +
				"/stop - остановить уведомления\n\n" +
				"🎨 NFT коллекции:\n" +
				"/nft <символ> - цена любой коллекции\n" +
				"/popular - популярные коллекции"

		case text == "/popular":
			msgText = "🌟 Популярные коллекции:\n\n" +
				"• mad_lads - Mad Lads\n" +
				"• degods - DeGods\n" +
				"• famous_fox_federation - Famous Fox\n" +
				"• solana_monkey_business - Solana Monkey"

		case text == "/btc":
			price, err := getCryptoPriceWithCache("bitcoin")
			if err != nil {
				if strings.Contains(err.Error(), "превышен лимит") {
					msgText = "❌ Сервис временно недоступен из-за большого количества запросов\nПопробуйте через 5-10 минут"
				} else {
					msgText = "❌ Временная ошибка получения цены\nПопробуйте позже"
				}
				log.Printf("Ошибка получения BTC: %v", err)
			} else {
				msgText = fmt.Sprintf("💰 Bitcoin: $%.2f", price)
			}

		case text == "/zec":
			price, err := getCryptoPriceWithCache("zcash")
			if err != nil {
				if strings.Contains(err.Error(), "превышен лимит") {
					msgText = "❌ Сервис временно недоступен из-за большого количества запросов\nПопробуйте через 5-10 минут"
				} else {
					msgText = "❌ Временная ошибка получения цены\nПопробуйте позже"
				}
				log.Printf("Ошибка получения ZEC: %v