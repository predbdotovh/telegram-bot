package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"strings"

	"gopkg.in/telegram-bot-api.v4"
)

type configuration struct {
	Token       string
	WebhookHost string
	WebhookRoot string
	LocalListen string
}

func main() {
	log.Print("Hello")

	log.Print("Open file")
	file, err := os.Open("config.json")
	if err != nil {
		log.Fatal(err)
	}

	log.Print("Decode file")
	decoder := json.NewDecoder(file)
	conf := configuration{}
	err = decoder.Decode(&conf)
	if err != nil {
		log.Fatal(err)
	}

	log.Print("Init bot API")
	bot, err := tgbotapi.NewBotAPI(conf.Token)
	if err != nil {
		log.Fatal(err)
	}

	bot.Debug = true

	log.Printf("Authorized on account %s", bot.Self.UserName)

	_, err = bot.SetWebhook(tgbotapi.NewWebhook(conf.WebhookHost + conf.WebhookRoot + bot.Token))
	if err != nil {
		log.Fatal(err)
	}

	log.Print("Listen for webhook")
	updates := bot.ListenForWebhook(conf.WebhookRoot + bot.Token)
	go http.ListenAndServe(conf.LocalListen, nil)

	for update := range updates {
		if update.Message != nil {
			handleMessage(bot, update.Message)
		} else if update.EditedMessage != nil {
			handleMessage(bot, update.EditedMessage)
		} else if update.InlineQuery != nil {
			handleInline(bot, update.InlineQuery)
		} else {
			log.Printf("%+v\n", update)
		}
	}
}

func queryAPI(str string) ([]string, error) {
	return nil, nil
}

func handleInline(bot *tgbotapi.BotAPI, iq *tgbotapi.InlineQuery) {
	log.Printf("%+v\n", iq)

	res, err := queryAPI(iq.Query)
	if err != nil {
		log.Print(err)
		return
	}
	log.Print(res)

	answer := tgbotapi.InlineConfig{
		InlineQueryID: iq.ID,
		Results: []interface{}{
			tgbotapi.NewInlineQueryResultArticle("0", "Title", "Content"),
		},
	}
	response, err := bot.AnswerInlineQuery(answer)
	log.Print(response)
	if err != nil {
		log.Print(err)
	}
}

func handleMessage(bot *tgbotapi.BotAPI, m *tgbotapi.Message) {
	log.Printf("%+v\n", m)

	if len(m.Text) == 0 {
		return
	}

	if m.IsCommand() {
		handleCommand(bot, m, m.Command(), m.CommandArguments())
		return
	}

	// Don't bother with group messages
	if !m.Chat.IsPrivate() {
		return
	}

	res, err := queryAPI(m.Text)
	if err != nil {
		log.Print(err)
		return
	}

	log.Print(res)
}

func handleCommand(bot *tgbotapi.BotAPI, m *tgbotapi.Message, command, args string) {
	if !(m.Chat.IsPrivate() || strings.HasPrefix(m.Text, "/"+command+"@"+bot.Self.UserName)) {
		return
	}

	switch command {
	case "start":
		handleCommandStart(bot, m)
	case "help":
		handleCommandHelp(bot, m)
	case "ping":
		handleCommandPing(bot, m)
	case "query":
		handleCommandQuery(bot, m, args)
	default:
		handleCommandUnknown(bot, m)
	}
}

const startContent = `Hello !
This bot has inline mode activated, feel free to query me :
@PredbBot <query>
Type /help for available commands.`

func handleCommandStart(bot *tgbotapi.BotAPI, m *tgbotapi.Message) {
	// /start shouldn't happen outside of private
	if m.Chat.IsPrivate() {
		msg := tgbotapi.NewMessage(m.Chat.ID, startContent)
		bot.Send(msg)
	}
}

const helpContent = `/ping : Check if I'm still alive
/query <string> : Query for release name at predb.ovh`

func handleCommandHelp(bot *tgbotapi.BotAPI, m *tgbotapi.Message) {
	msg := tgbotapi.NewMessage(m.Chat.ID, helpContent)
	bot.Send(msg)
}

func handleCommandPing(bot *tgbotapi.BotAPI, m *tgbotapi.Message) {
	msg := tgbotapi.NewMessage(m.Chat.ID, "Pong")
	bot.Send(msg)
}

func handleCommandQuery(bot *tgbotapi.BotAPI, m *tgbotapi.Message, args string) {
	res, err := queryAPI(args)

	if err != nil {
		log.Print(err)
		return
	}

	msg := tgbotapi.NewMessage(m.Chat.ID, fmt.Sprintf("Results : %d", len(res)))
	bot.Send(msg)
}

func handleCommandUnknown(bot *tgbotapi.BotAPI, m *tgbotapi.Message) {
	msg := tgbotapi.NewMessage(m.Chat.ID, "I didn't understand that. List available commands with /help")
	bot.Send(msg)
}
