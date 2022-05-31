package protocol

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/rumblefrog/source-chat-relay/server/config"
	"github.com/rumblefrog/source-chat-relay/server/packet"
	"github.com/tidwall/gjson"
)

type IdentificationType uint8

const (
	IdentificationInvalid IdentificationType = iota
	IdentificationSteam
	IdentificationDiscord
	IdentificationTypeCount
)

type SteamAvatarType uint8

const (
	SteamAvatarSmall SteamAvatarType = iota
	SteamAvatarMedium
	SteamAvatarFull
)

type ChatMessage struct {
	BaseMessage

	IDType IdentificationType

	ID string

	Username string

	Message string
}

func ParseChatMessage(base BaseMessage, r *packet.PacketReader) (*ChatMessage, error) {
	m := &ChatMessage{}

	m.BaseMessage = base

	m.IDType = ParseIdentificationType(r.ReadUint8())

	var ok bool

	m.ID, ok = r.TryReadString()

	if !ok {
		return nil, ErrCannotReadString
	}

	m.Username, ok = r.TryReadString()

	if !ok {
		return nil, ErrCannotReadString
	}

	m.Message, ok = r.TryReadString()

	if !ok {
		return nil, ErrCannotReadString
	}

	return m, nil
}

func (m *ChatMessage) Type() MessageType {
	return MessageChat
}

func (m *ChatMessage) Content() string {
	return m.Message
}

func (m *ChatMessage) Marshal() []byte {
	var builder packet.PacketBuilder

	builder.WriteByte(byte(MessageChat))
	builder.WriteCString(m.BaseMessage.EntityName)

	builder.WriteByte(byte(m.IDType))
	builder.WriteCString(m.ID)
	builder.WriteCString(m.Username)
	builder.WriteCString(m.Message)

	return builder.Bytes()
}

func (m *ChatMessage) Plain() string {
	replacer := strings.NewReplacer("%username%", m.Username, "%message%", m.Message, "%id%", m.ID)

	return replacer.Replace(config.Config.Messages.EventFormatSimplePlayerChat)
}

func (m *ChatMessage) Embed() *discordgo.MessageEmbed {
	idColorBytes := []byte(m.ID)

	// Convert to an int with length of 6
	color := int(binary.LittleEndian.Uint32(idColorBytes[len(idColorBytes)-6:])) / 10000

	loc, err := time.LoadLocation(config.Config.General.TimeZone)

	timestamp := ""

	if err == nil {
		timestamp = time.Now().In(loc).Format(time.RFC3339)
	} else {
		timestamp = time.Now().Format(time.RFC3339)
	}

	switch m.IDType {
	case IdentificationSteam:
		avatarURL, err := m.SteamAvatarURL(SteamAvatarMedium)

		if err == nil {
			return &discordgo.MessageEmbed{
				Color:       color,
				Description: m.Message,
				Timestamp:   timestamp,
				Author: &discordgo.MessageEmbedAuthor{
					Name: m.Username,
					URL:  m.IDType.FormatURL(m.ID),
				},
				Thumbnail: &discordgo.MessageEmbedThumbnail{
					URL:    avatarURL,
					Width:  48,
					Height: 48,
				},
				Footer: &discordgo.MessageEmbedFooter{
					Text: fmt.Sprintf("%s | %s", m.BaseMessage.EntityName, m.ID),
				},
			}
		}

		fallthrough
	default:
		return &discordgo.MessageEmbed{
			Color:       color,
			Description: m.Message,
			Timestamp:   timestamp,
			Author: &discordgo.MessageEmbedAuthor{
				Name: m.Username,
				URL:  m.IDType.FormatURL(m.ID),
			},
			Footer: &discordgo.MessageEmbedFooter{
				Text: fmt.Sprintf("%s | %s", m.BaseMessage.EntityName, m.ID),
			},
		}
	}
}

func (m *ChatMessage) SteamAvatarURL(t SteamAvatarType) (string, error) {
	if m.IDType != IdentificationSteam {
		return "", errors.New("avatar is currently supported on only steam")
	}

	resp, err := http.Get(fmt.Sprintf("https://api.steampowered.com/ISteamUser/GetPlayerSummaries/v2/?key=%s&steamids=%s", config.Config.Secrets.SteamAPIKey, m.ID))
	if err != nil {
		return "", err
	}

	defer resp.Body.Close()

	rawJSON, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	switch t {
	case SteamAvatarSmall:
		return gjson.Get(string(rawJSON), "response.players.0.avatar").String(), nil
	case SteamAvatarMedium:
		return gjson.Get(string(rawJSON), "response.players.0.avatarmedium").String(), nil
	case SteamAvatarFull:
		return gjson.Get(string(rawJSON), "response.players.0.avatarfull").String(), nil
	default:
		return gjson.Get(string(rawJSON), "response.players.0.avatar").String(), nil
	}
}

func ParseIdentificationType(t uint8) IdentificationType {
	if t >= uint8(IdentificationTypeCount) {
		return IdentificationInvalid
	}

	return IdentificationType(t)
}

func (i IdentificationType) FormatURL(id string) string {
	switch i {
	case IdentificationSteam:
		return "https://steamcommunity.com/profiles/" + id
	default:
		return ""
	}
}
