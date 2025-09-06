package commands

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"Twilight/queue"
	"Twilight/yt"

	"github.com/bwmarrin/discordgo"
	"github.com/kkdai/youtube/v2"
)

// playSong plays the song given a link, adding the song to the song queue
func playSong(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate) *interactionError {
	// Get user's current voice channel
	vs, err := s.State.VoiceState(i.GuildID, i.Member.User.ID)
	if err != nil || vs == nil || vs.ChannelID == "" {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Join a voice channel first 😉",
			},
		})
		return nil
	}

	// Check if bot is already in a voice channel
	if vc, ok := s.VoiceConnections[i.GuildID]; ok && vc != nil {
		if vc.ChannelID != vs.ChannelID {
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "I'm already in another voice channel 😅",
				},
			})
			return nil
		}
	}

	videoURL := i.ApplicationCommandData().Options[0].StringValue()
	videoID, err := youtube.ExtractVideoID(videoURL)
	if err != nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "❌ Invalid YouTube link!"},
		})
		return nil
	}

	client := youtube.Client{}
	_, err = client.GetVideo(videoID)
	if err != nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "❌ Could not fetch the video. It may be private or removed."},
		})
		return nil
	}

	currentVideo, _ := yt.FetchVideoMetadata(videoID)
	err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("🎵 Adding **%s** to the queue...", currentVideo.Title),
		},
	})
	if err != nil {
		return &interactionError{err: err, message: "Failed to respond"}
	}

	vc, err := connectUserVoiceChannel(s, i.GuildID, i.Member.User.ID)
	if err != nil {
		return nil
	}

	os.MkdirAll("cache", 0755)
	filename := fmt.Sprintf("cache/%s.mp3", videoID)
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		stream, err := yt.FetchVideoStream(videoID)
		if err != nil {
			return nil
		}

		out, err := os.Create(filename)
		if err != nil {
			stream.Close()
			return nil
		}

		_, err = io.Copy(out, stream)
		out.Close()
		stream.Close()
		if err != nil {
			os.Remove(filename)
			return nil
		}
	}

	gq := queue.Enqueue(i.GuildID, filename, i.Member.User.Username)
	if gq.Session.VC == nil {
		go queue.PlayNext(s, i.GuildID, vc)
	}

	return nil
}

// pauseSong pauses the current song
func pauseSong(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate) *interactionError {
	gq, ok := queue.GetGuildQueue(i.GuildID)
	if !ok || gq.Session.VC == nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "Nothing is playing right now 😶"},
		})
		return nil
	}
	gq.Session.Pause()
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Content: "⏸️ Paused"},
	})
	return nil
}

// resumeSong resumes the current song
func resumeSong(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate) *interactionError {
	gq, ok := queue.GetGuildQueue(i.GuildID)
	if !ok || gq.Session.VC == nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "Nothing is playing right now 😶"},
		})
		return nil
	}
	gq.Session.Resume()
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Content: "▶️ Resumed"},
	})
	return nil
}

// stopSong stops the current session and disconnects the bot from the voice channel
func stopSong(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate) *interactionError {
	gq, ok := queue.GetGuildQueue(i.GuildID)
	if !ok {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "Nothing is playing right now 😶"},
		})
		return nil
	}

	gq.Session.Stop()
	if gq.Session.VC != nil {
		gq.Session.VC.Disconnect()
	}

	queue.ClearCurrentItem(i.GuildID)

	queue.DeleteGuildQueue(i.GuildID)
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Content: "⏹️ Stopped"},
	})
	return nil
}

// skipSong skips the current song playing and moves on to the next
func skipSong(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate) *interactionError {
	gq, ok := queue.GetGuildQueue(i.GuildID)
	if !ok || gq.Session.VC == nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "Nothing is playing right now 😶"},
		})
		return nil
	}

	gq.Session.Stop()

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Content: "⏭️ Skipped"},
	})

	return nil
}

// currentSong displays the current song being played
func currentSong(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate) *interactionError {
	gq, ok := queue.GetGuildQueue(i.GuildID)
	if !ok || gq.Session.VC == nil || gq.CurrentItem == nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "🎶 Nothing is playing right now 😶"},
		})
		return nil
	}

	currentItem := gq.CurrentItem
	status := "▶️ Playing"
	if gq.Session.IsPaused {
		status = "⏸️ Paused"
	}

	currentID := strings.TrimSuffix(strings.TrimPrefix(currentItem.Filename, "cache/"), ".mp3")
	currentVideo, _ := yt.FetchVideoMetadata(currentID)

	thumbnailURL := ""
	if len(currentVideo.Thumbnails) > 0 {
		thumbnailURL = currentVideo.Thumbnails[0].URL
	}
	videoURL := fmt.Sprintf("https://www.youtube.com/watch?v=%s", currentID)

	embed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("🎵 Now Playing: %s", currentVideo.Title),
		URL:         videoURL,
		Description: fmt.Sprintf("Requested by: %s\nStatus: %s", currentItem.RequestedBy, status),
		Thumbnail:   &discordgo.MessageEmbedThumbnail{URL: thumbnailURL},
	}

	if len(gq.Items) > 0 {
		queueText := "**Up Next:**\n"
		queueLimit := len(gq.Items)
		if queueLimit > 5 {
			queueLimit = 5
		}
		for idx, item := range gq.Items[:queueLimit] {
			itemID := strings.TrimSuffix(strings.TrimPrefix(item.Filename, "cache/"), ".mp3")
			video, _ := yt.FetchVideoMetadata(itemID)
			queueText += fmt.Sprintf("%d. `%s` (requested by %s)\n", idx+1, video.Title, item.RequestedBy)
		}
		if len(gq.Items) > 5 {
			queueText += fmt.Sprintf("...and %d more", len(gq.Items)-5)
		}
		embed.Fields = []*discordgo.MessageEmbedField{
			{
				Name:  "Queue",
				Value: queueText,
			},
		}
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	})
	return nil
}

// currentQueue shows the list of songs in the queue using an embed
func currentQueue(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate) *interactionError {
	gq, ok := queue.GetGuildQueue(i.GuildID)
	if !ok || gq.Session.VC == nil || gq.CurrentItem == nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "🎶 The queue is empty 😶"},
		})
		return nil
	}

	embed := &discordgo.MessageEmbed{
		Title: fmt.Sprintf("🎶 Queue for %s", i.GuildID),
	}

	queueText := ""
	currentID := strings.TrimSuffix(strings.TrimPrefix(gq.CurrentItem.Filename, "cache/"), ".mp3")
	currentVideo, _ := yt.FetchVideoMetadata(currentID)
	queueText += fmt.Sprintf("1. `%s` (requested by %s) ▶️\n", currentVideo.Title, gq.CurrentItem.RequestedBy)

	for idx, item := range gq.Items {
		itemID := strings.TrimSuffix(strings.TrimPrefix(item.Filename, "cache/"), ".mp3")
		video, _ := yt.FetchVideoMetadata(itemID)
		queueText += fmt.Sprintf("%d. `%s` (requested by %s)\n", idx+2, video.Title, item.RequestedBy)
	}

	embed.Fields = []*discordgo.MessageEmbedField{
		{
			Name:  "Queue",
			Value: queueText,
		},
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	})
	return nil
}
