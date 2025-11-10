package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Структура для цены биткоина
type BitcoinPrice struct {
	Price float64 `json:"usd"`
}

// Структура для NFT с Magic Eden
type NFTStats struct {
	Symbol      string  `json:"symbol"`
	FloorPrice  int64   `json:"floorPrice"`
	ListedCount int     `json:"listedCount"`
	VolumeAll   float64 `json:"volumeAll"`
}

// Функция для получения цены биткоина
func getBitcoinPrice() (float64, error) {
	url := "https://api.coingecko.com/api/v3/simple/price?ids=bitcoin&vs_currencies=usd"

	resp, err := http.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	var result map[string]BitcoinPrice
	err = json.Unmarshal(body, &result)
	if err != nil {
		return 0, err
	}

	return result["bitcoin"].Price, nil
}

// Функция для получения цены NFT коллекции
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

// Красивое форматирование имени коллекции
func formatCollectionName(symbol string) string {
	name := strings.ReplaceAll(symbol, "_", " ")
	name = strings.Title(name)
	return name
}

func main() {
	bot, err := tgbotapi.NewBotAPI("8569683760:AAEXxy5gFvKYeiP7LNo4Oil6PbmuIORzbKs") // Токен уже вставлен!
	if err != nil {
		log.Panic(err)
	}

	bot.Debug = true
	log.Printf("✅ Бот %s запущен! 🚀", bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		if update.Message == nil {
			continue
		}

		var msgText string
		text := strings.TrimSpace(update.Message.Text)

		switch {
		case text == "/start":
			msgText = "👋 **Crypto & NFT Tracker Bot**\n\n" +
				"💰 **Криптовалюты:**\n" +
				"/btc - цена Bitcoin\n\n" +
				"🎨 **NFT коллекции:**\n" +
				"/nft <символ> - цена любой коллекции\n" +
				"/popular - популярные коллекции\n\n" +
				"**Примеры:**\n" +
				"`/nft mad_lads`\n" +
				"`/nft degods`\n" +
				"`/nft solana_monkey_business`"

		case text == "/popular":
			msgText = "🌟 **Популярные коллекции:**\n\n" +
				"• `mad_lads` - Mad Lads\n" +
				"• `degods` - DeGods\n" +
				"• `famous_fox_federation` - Famous Fox\n" +
				"• `solana_monkey_business` - Solana Monkey\n\n" +
				"Используй: `/nft символ`"

		case text == "/btc":
			price, err := getBitcoinPrice()
			if err != nil {
				msgText = "❌ Ошибка получения цены BTC"
			} else {
				msgText = fmt.Sprintf("💰 **Bitcoin**: $%.2f", price)
			}

		case strings.HasPrefix(text, "/nft "):
			collectionSymbol := strings.TrimPrefix(text, "/nft ")
			if collectionSymbol == "" {
				msgText = "❌ Укажи символ коллекции\nПример: `/nft mad_lads`"
			} else {
				stats, err := getNFTPrice(collectionSymbol)
				if err != nil {
					msgText = fmt.Sprintf("❌ Коллекция '%s' не найдена", collectionSymbol)
				} else {
					floorPriceSOL := float64(stats.FloorPrice) / 1_000_000_000
					msgText = fmt.Sprintf("🎨 **%s**\n\n🏷️ **Floor Price:** %.2f SOL\n📊 **Listed:** %d NFTs",
						formatCollectionName(collectionSymbol), floorPriceSOL, stats.ListedCount)
				}
			}

		default:
			msgText = "Напиши /start для списка команд 🚀"
		}

		msg := tgbotapi.NewMessage(update.Message.Chat.ID, msgText)
		msg.ParseMode = "Markdown"
		bot.Send(msg)
	}
}
