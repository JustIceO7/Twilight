package commands

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"Twilight/queue"
	"Twilight/utils"
	"Twilight/yt"

	"github.com/bwmarrin/discordgo"
	"github.com/kkdai/youtube/v2"
	"github.com/spf13/viper"
)

// playSong plays the song given a link, adding the song to the song queue
func playSong(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate) *interactionError {
	// Check if user is in a voice channel and bot is not in a different one
	if !checkUserVoiceChannel(s, i) {
		return nil
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

	vc, err := connectUserVoiceChannel(s, i.GuildID, i.Member.User.ID)
	if err != nil {
		return nil
	}

	os.MkdirAll("cache", 0755)
	filename := fmt.Sprintf("cache/%s.mp3", videoID)
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		stream, err := yt.FetchVideoStream(videoID)
		if err != nil {
			fmt.Printf("DEBUG: FetchVideoStream error: %v\n", err)
			sendErrorResponse(s, i)
			return nil
		}

		out, err := os.Create(filename)
		if err != nil {
			fmt.Printf("DEBUG: Create file error: %v\n", err)
			stream.Close()
			sendErrorResponse(s, i)
			return nil
		}

		_, err = io.Copy(out, stream)
		out.Close()
		stream.Close()
		if err != nil {
			fmt.Printf("DEBUG: Copy error: %v\n", err)
			os.Remove(filename)
			sendErrorResponse(s, i)
			return nil
		}
	}

	currentVideo, _ := yt.FetchVideoMetadata(videoID)
	err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("🎵 **%s** added to the queue (`%s`)", currentVideo.Title, utils.FormatYtDuration(currentVideo.Duration)),
		},
	})
	if err != nil {
		return &interactionError{err: err, message: "Failed to respond"}
	}

	gq := queue.Enqueue(i.GuildID, filename, i.Member.User.Username)
	if gq.Session.VC == nil {
		go queue.PlayNext(s, i.GuildID, vc)
	}

	return nil
}

// pauseSong pauses the current song
func pauseSong(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate) *interactionError {
	// Check if user is in a voice channel and bot is not in a different one
	if !checkUserVoiceChannel(s, i) {
		return nil
	}

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
	// Check if user is in a voice channel and bot is not in a different one
	if !checkUserVoiceChannel(s, i) {
		return nil
	}

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
	// Check if user is in a voice channel and bot is not in a different one
	if !checkUserVoiceChannel(s, i) {
		return nil
	}

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
		Data: &discordgo.InteractionResponseData{Content: "⏹️ Playback stopped and disconnected"},
	})
	return nil
}

// skipSong skips the current song playing and moves on to the next
func skipSong(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate) *interactionError {
	// Check if user is in a voice channel and bot is not in a different one
	if !checkUserVoiceChannel(s, i) {
		return nil
	}

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
	// Check if user is in a voice channel and bot is not in a different one
	if !checkUserVoiceChannel(s, i) {
		return nil
	}

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
		Color:       viper.GetInt("theme"),
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
	// Check if user is in a voice channel and bot is not in a different one
	if !checkUserVoiceChannel(s, i) {
		return nil
	}

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
		Color: viper.GetInt("theme"),
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

// shuffleQueue shuffles the current song queue
func shuffleQueue(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate) *interactionError {
	// Check if user is in a voice channel and bot is not in a different one
	if !checkUserVoiceChannel(s, i) {
		return nil
	}
	gq, ok := queue.GetGuildQueue(i.GuildID)
	if !ok || gq.Session.VC == nil || len(gq.Items) == 0 {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "🎶 The queue is empty 😶"},
		})
		return nil
	}
	queue.ShuffleGuildQueue(i.GuildID)
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Content: "🔀 Queue shuffled!"},
	})
	return nil
}
