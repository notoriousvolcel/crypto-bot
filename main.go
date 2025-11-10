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
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Структуры для цен
type CryptoPrice struct {
	USD float64 `json:"usd"`
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
var activeChats = make(map[int64]bool)

// Функции для получения цен
func getCryptoPrice(coin string) (float64, error) {
	url := fmt.Sprintf("https://api.coingecko.com/api/v3/simple/price?ids=%s&vs_currencies=usd", coin)

	resp, err := http.Get(url)
	if err != nil {
		return 0, fmt.Errorf("ошибка запроса: %v", err)
	}
	defer resp.Body.Close()

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

func getNFTPrice(collectionSymbol string) (*NFTStats, error) {
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
	ticker := time.NewTicker(30 * time.Second)

	go func() {
		for range ticker.C {
			for chatID, settings := range notificationSettings {
				if !settings.Enabled {
					continue
				}

				price, err := getCryptoPrice("zcash")
				if err != nil {
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

				time.Sleep(settings.Interval)
			}
		}
	}()
}

// Функция для шуточных уведомлений (неотключаемая)
func startJokeNotifications(bot *tgbotapi.BotAPI) {
	ticker := time.NewTicker(1 * time.Minute)

	go func() {
		for range ticker.C {
			for chatID := range activeChats {
				jokeMessages := []string{
					"Ты пидор! 😄",
				}

				randomIndex := rand.Intn(len(jokeMessages))
				message := jokeMessages[randomIndex]

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

	// Запускаем шуточные уведомления
	startJokeNotifications(bot)

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
			// Активируем шуточные уведомления (неотключаемые)
			activeChats[chatID] = true

			msgText = "👋 Crypto & NFT Tracker Bot\n\n" +
				"💰 Криптовалюты:\n" +
				"/btc - цена Bitcoin\n" +
				"/zec - цена Zcash\n" +
				"/notify_zec - уведомления ZEC (интервал: 2 мин)\n" +
				"/interval <время> - изменить интервал\n" +
				"/stop - остановить уведомления\n\n" +
				"🎨 NFT коллекции:\n" +
				"/nft <символ> - цена любой коллекции\n" +
				"/popular - популярные коллекции\n\n" +

		case text == "/popular":
			msgText = "🌟 Популярные коллекции:\n\n" +
				"• mad_lads - Mad Lads\n" +
				"• degods - DeGods\n" +
				"• famous_fox_federation - Famous Fox\n" +
				"• solana_monkey_business - Solana Monkey"

		case text == "/btc":
			price, err := getCryptoPrice("bitcoin")
			if err != nil {
				msgText = "❌ Ошибка получения цены BTC: " + err.Error()
			} else {
				msgText = fmt.Sprintf("💰 Bitcoin: $%.2f", price)
			}

		case text == "/zec":
			price, err := getCryptoPrice("zcash")
			if err != nil {
				msgText = "❌ Ошибка получения цены ZEC: " + err.Error()
			} else {
				msgText = fmt.Sprintf("🛡️ Zcash: $%.2f", price)
			}

		case text == "/notify_zec":
			if settings, exists := notificationSettings[chatID]; exists {
				settings.Enabled = true
			} else {
				notificationSettings[chatID] = &NotificationSettings{
					Enabled:  true,
					Interval: 2 * time.Minute,
				}
			}
			msgText = fmt.Sprintf("✅ Уведомления ZEC включены!\nИнтервал: %v", notificationSettings[chatID].Interval)

		case text == "/stop":
			if settings, exists := notificationSettings[chatID]; exists {
				settings.Enabled = false
				msgText = "⏹️ Уведомления ZEC остановлены\n" +
					"⚠️ Шуточные уведомления продолжают работать! 😄"
			} else {
				msgText = "ℹ️ Уведомления ZEC не были включены\n" +
					"⚠️ Шуточные уведомления работают! 😄"
			}

		case strings.HasPrefix(text, "/interval "):
			intervalStr := strings.TrimPrefix(text, "/interval ")
			interval, err := parseInterval(intervalStr)
			if err != nil {
				msgText = fmt.Sprintf("❌ %s", err.Error())
			} else {
				if settings, exists := notificationSettings[chatID]; exists {
					settings.Interval = interval
				} else {
					notificationSettings[chatID] = &NotificationSettings{
						Enabled:  false,
						Interval: interval,
					}
				}
				msgText = fmt.Sprintf("✅ Интервал уведомлений установлен: %v\nИспользуйте /notify_zec для включения", interval)
			}

		case strings.HasPrefix(text, "/nft "):
			collectionSymbol := strings.TrimPrefix(text, "/nft ")
			if collectionSymbol == "" {
				msgText = "❌ Укажи символ коллекции\nПример: /nft mad_lads"
			} else {
				stats, err := getNFTPrice(collectionSymbol)
				if err != nil {
					msgText = fmt.Sprintf("❌ Коллекция '%s' не найдена", collectionSymbol)
				} else {
					floorPriceSOL := float64(stats.FloorPrice) / 1_000_000_000
					msgText = fmt.Sprintf("🎨 %s\n\n🏷️ Floor Price: %.2f SOL\n📊 Listed: %d NFTs",
						formatCollectionName(collectionSymbol), floorPriceSOL, stats.ListedCount)
				}
			}

		default:
			msgText = "Напиши /start для списка команд 🚀"
		}

		msg := tgbotapi.NewMessage(chatID, msgText)
		bot.Send(msg)
	}
}

// Функция для получения токена
func getToken() string {
	token := os.Getenv("TELEGRAM_TOKEN")
	if token == "" {
		log.Fatal("TELEGRAM_TOKEN не установлен")
	}
	return token
}
 
 
