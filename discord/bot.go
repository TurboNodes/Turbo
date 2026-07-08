package discord

import (
	"bytes"
	"fmt"
	"log"
	"server/proxy"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

var (
	commands = []*discordgo.ApplicationCommand{
		{
			Name:        "screenshot",
			Description: "Get a screenshot of the given website URL",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "url",
					Description: "The website URL to screenshot",
					Required:    true,
				},
			},
		},
	}

	commandHandlers = map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate){
		"screenshot": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			startTime := time.Now()

			urlStr := i.ApplicationCommandData().Options[0].StringValue()
			if !strings.HasPrefix(urlStr, "http://") && !strings.HasPrefix(urlStr, "https://") {
				urlStr = "https://" + urlStr
			}

			err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
			})
			if err != nil {
				log.Println("Error deferring interaction:", err)
				return
			}

			client := proxy.FindClient()
			if client == nil {
				s.FollowupMessageCreate(i.Interaction, false, &discordgo.WebhookParams{
					Content: "❌ No clients available, try again later",
				})
				return
			}
			err = client.SendMessage(proxy.Message{
				Type: "browser_screenshot",
				ID:   "1",
				Addr: urlStr,
			})
			if err != nil {
				log.Println("sending browser screenshot request failed:", err)
				s.FollowupMessageCreate(i.Interaction, false, &discordgo.WebhookParams{
					Content: "❌ Failed to process screenshot request",
				})
				return
			}

			var data []byte
			select {
			case data = <-proxy.BrowserScreenshotData:
				_, err = s.FollowupMessageCreate(i.Interaction, false, &discordgo.WebhookParams{
					Content: fmt.Sprintf("Here is the screenshot for [your website](%s).", urlStr),
					Files: []*discordgo.File{
						{
							Name:   "screenshot.png",
							Reader: bytes.NewReader(data),
						},
					},
					Embeds: []*discordgo.MessageEmbed{
						{
							Image: &discordgo.MessageEmbedImage{
								URL: "attachment://screenshot.png",
							},
							Description: fmt.Sprintf("*took %s*", time.Since(startTime)),
						},
					},
					Components: []discordgo.MessageComponent{
						discordgo.ActionsRow{
							Components: []discordgo.MessageComponent{
								discordgo.Button{
									Label:    "Report",
									Style:    discordgo.DangerButton,
									CustomID: "report_open",
									Emoji: &discordgo.ComponentEmoji{
										Name: "⚠️",
									},
								},
							},
						},
					},
				})
				if err != nil {
					log.Println("Error sending screenshot followup:", err)
					return
				}

			case <-time.After(30 * time.Second):
				_, err = s.FollowupMessageCreate(i.Interaction, false, &discordgo.WebhookParams{
					Content: "⏰ Screenshot request timed out after 30 seconds. The website may be slow or unreachable.",
				})
				if err != nil {
					log.Println("Error sending timeout followup:", err)
				}
				return
			}

		},
	}
	guildID = "1366473367791861770"
)

func StartBot(token string) {
	session, err := discordgo.New("Bot " + token)
	if err != nil {
		log.Println("Error creating Discord session:", err)
	}
	session.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		// Only handle application command interactions here
		if i.Type == discordgo.InteractionApplicationCommand {
			if handler, ok := commandHandlers[i.ApplicationCommandData().Name]; ok {
				handler(s, i)
			}
		}
	})
	session.AddHandler(interactionCreate)

	err = session.Open()
	if err != nil {
		log.Println("Cannot open Discord session:", err)
	}

	registeredCommands := make([]*discordgo.ApplicationCommand, len(commands))
	for i, v := range commands {
		cmd, err := session.ApplicationCommandCreate(session.State.User.ID, guildID, v)
		if err != nil {
			log.Panicf("Cannot create '%v' command: %v", v.Name, err)
		}
		registeredCommands[i] = cmd
	}

	//defer session.Close()

}

func interactionCreate(s *discordgo.Session, i *discordgo.InteractionCreate) {

	// Only care about component interactions
	if i.Type != discordgo.InteractionMessageComponent {
		return
	}

	data := i.MessageComponentData()

	switch data.CustomID {

	// 🔘 BUTTON CLICK (#2)
	case "report_open":
		err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Please select the issue:",
				Components: []discordgo.MessageComponent{
					discordgo.ActionsRow{
						Components: []discordgo.MessageComponent{
							discordgo.SelectMenu{
								CustomID:    "report_reason",
								Placeholder: "Select the issue",
								Options: []discordgo.SelectMenuOption{
									{Label: "NSFW / Illegal content", Value: "nsfw"},
									{Label: "Website blocked the bot", Value: "Blocked request"},
									{Label: "Page not rendered properly", Value: "Rendering issue"},
								},
							},
						},
					},
				},
				Flags: discordgo.MessageFlagsEphemeral,
			},
		})
		if err != nil {
			log.Println(err)
		}

	// 📋 SELECT MENU CHOICE (#3)
	case "report_reason":
		reason := data.Values[0]

		jumpURL := fmt.Sprintf(
			"https://discord.com/channels/%s/%s/%s",
			i.GuildID,
			i.ChannelID,
			i.Message.ID,
		)

		_, err := s.ChannelMessageSend("1456400539372884091", fmt.Sprintf("🚨 **New Report** 🚨\nReporter: <@%s>\nReason: %s\nMessage: %s", i.Member.User.ID, reason, jumpURL))
		if err != nil {
			log.Println("Error sending report message:", err)
		}

		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseUpdateMessage,
			Data: &discordgo.InteractionResponseData{
				Content: "✅ Thank you for the report. It has been recorded.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
	}
}
