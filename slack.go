//lint:file-ignore SA4006 ignore unused variable warning
package main

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/acarl005/stripansi"
	"github.com/quackduck/term"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

var (
	SlackChan   chan string
	SlackAPI    *slack.Client
	SlackSocket *socketmode.Client
	SlackBotID  string
)

func getMsgsFromSlack() {
	if Integrations.Slack == nil {
		return
	}

	fmt.Printf("Slack: getMsgsFromSlack\n")

	uslack := new(User)
	uslack.isBridge = true
	devnull, _ := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	uslack.term = term.NewTerminal(devnull, "")
	uslack.room = MainRoom
	for ev := range SlackSocket.Events {
		fmt.Printf("Recieved event\n")
		switch ev.Type {
		case socketmode.EventTypeEventsAPI:
			fmt.Printf("Recieved actual event\n")
			eventsAPIEvent, ok := ev.Data.(slackevents.EventsAPIEvent)
			if !ok {
				continue
			}

			SlackSocket.Ack(*ev.Request)
			fmt.Printf("Acked request\n")

			switch eventsAPIEvent.Type {
			case slackevents.CallbackEvent:
				innerEvent := eventsAPIEvent.InnerEvent
				switch ev := innerEvent.Data.(type) {
				case *slackevents.MessageEvent:
					fmt.Printf("It's a message!\n")
					msg := ev
					if msg.SubType != "" {
						break // We're only handling normal messages.
					}

					text := msg.Text

					u, _ := SlackAPI.GetUserInfo(msg.User)
					if u == nil || u.ID == SlackBotID {
						break
					}

					h := sha1.Sum([]byte(u.ID))
					i, _ := strconv.ParseInt(hex.EncodeToString(h[:2]), 16, 0) // two bytes as an int
					name := strings.Fields(u.RealName)[0]
					uslack.Name = Yellow.Paint(Integrations.Slack.Prefix+" ") + (Styles[int(i)%len(Styles)]).apply(name)
					if Integrations.Discord != nil {
						DiscordChan <- DiscordMsg{
							senderName: Integrations.Slack.Prefix + " " + name,
							msg:        text,
							channel:    uslack.room.name,
						} // send this discord message to slack
					}
					fmt.Printf("YEAG\n")
					runCommands(text, uslack)
				}
			default:
				SlackAPI.Debugf("unsupported Events API event received")
			}
		case socketmode.EventTypeConnected:
			authTest, err := SlackAPI.AuthTest()
			if err != nil {
				Log.Println("Failed to authenticate with Slack:", err)
				return
			}
			SlackBotID = authTest.UserID
			Log.Println("Connected to Slack with bot ID", SlackBotID, "as", authTest.User)
		case socketmode.EventTypeInvalidAuth:
			Log.Println("Invalid Slack authentication")
			return
		}
	}
}

func slackInit() { // called by init() in config.go
	fmt.Printf("Slack integration was init\n")
	if Integrations.Slack == nil {
		fmt.Printf("Slack integration was cancelled\n")

		return
	}

	fmt.Printf("Slack integration was enabled\n")

	SlackAPI = slack.New(
		Integrations.Slack.Token,
		slack.OptionDebug(true),
		slack.OptionLog(log.New(os.Stderr, "slack-go/slack: ", log.LstdFlags|log.Lshortfile)),
		slack.OptionAppLevelToken(Integrations.Slack.SocketModeToken),
	)

	SlackSocket = socketmode.New(
		SlackAPI,
		socketmode.OptionDebug(true),
		socketmode.OptionLog(log.New(os.Stderr, "slack-go/slack/socketmode: ", log.Lshortfile|log.LstdFlags)),
	)
	SlackChan = make(chan string, 100)
	go func() {
		for msg := range SlackChan {
			msg = strings.ReplaceAll(stripansi.Strip(msg), `\n`, "\n")
			_, _, err := SlackAPI.PostMessage(
				Integrations.Slack.ChannelID,
				slack.MsgOptionText(msg, false),
			)
			if err != nil {
				log.Printf("Failed to send message to Slack: %v", err)
			}
		}
	}()

	go SlackSocket.Run()
}
