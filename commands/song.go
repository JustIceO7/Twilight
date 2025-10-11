package commands

import (
	"context"
	"fmt"
	"os"

	"Twilight/playlist"
	"Twilight/queue"
	"Twilight/redis_client"
	"Twilight/utils"
	"Twilight/yt"

	"github.com/bwmarrin/discordgo"
	"github.com/kkdai/youtube/v2"
	"github.com/spf13/viper"
)

// songInfo returns the metadata of a given song
func songInfo(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate) *interactionError {
	videoURL := i.ApplicationCommandData().Options[0].StringValue()
	videoID, err := youtube.ExtractVideoID(videoURL)
	if err != nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "❌ Invalid YouTube link!"},
		})
		return nil
	}
	ytManager := yt.NewYouTubeManager(redis_client.RDB)

	videoMetadata, err := ytManager.GetVideoMetadata(videoID)
	if err != nil {
		sendFetchErrorResponse(s, i)
		return nil
	}
	embed := &discordgo.MessageEmbed{
		Title:       videoMetadata.Title,
		URL:         videoURL,
		Description: videoMetadata.Description,
		Thumbnail:   &discordgo.MessageEmbedThumbnail{URL: videoMetadata.Thumbnails[0].URL},
		Color:       viper.GetInt("theme"),
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	})
	return nil
}

// playSong plays the song given a link, adding the song to the song queue
func playSong(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate) *interactionError {
	// Check if user is in a voice channel and bot is not in a different one
	if !checkUserVoiceChannel(s, i) {
		return nil
	}

	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	videoURL := i.ApplicationCommandData().Options[0].StringValue()
	videoID, err := youtube.ExtractVideoID(videoURL)
	if err != nil {
		s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
			Content: "❌ Invalid YouTube link!",
		})
		return nil
	}

	vc, err := connectUserVoiceChannel(s, i.GuildID, i.Member.User.ID)
	if err != nil {
		return nil
	}
	ytManager := yt.NewYouTubeManager(redis_client.RDB)

	filename := utils.GetAudioFile(videoID)
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		err := ytManager.DownloadAudio(videoID)
		if err != nil {
			fmt.Printf("DEBUG: Download error: %v\n", err)
			s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
				Content: "❌ Could not fetch the video. It may be private or removed.",
			})
			return nil
		}
	}

	currentVideo, err := ytManager.GetVideoMetadata(videoID)
	if err != nil {
		s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
			Content: "❌ Could not fetch video metadata.",
		})
		return nil
	}

	s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
		Content: fmt.Sprintf("🎵 **%s** added to the queue (`%s`)", currentVideo.Title, utils.FormatYtDuration(currentVideo.Duration)),
	})

	gq := queue.Enqueue(i.GuildID, filename, i.Member.User.Username)
	if gq.Session.VC == nil {
		go queue.PlayNext(s, i.GuildID, vc)
	}

	return nil
}

// playPlaylistSong plays an entire playlist adding to song queue
func playPlaylistSong(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate) *interactionError {
	// Check if user is in a voice channel and bot is not in a different one
	if !checkUserVoiceChannel(s, i) {
		return nil
	}

	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	ytManager := yt.NewYouTubeManager(redis_client.RDB)

	videoURL := i.ApplicationCommandData().Options[0].StringValue()
	videoIDs, err := ytManager.GetPlaylistVideoIDs(videoURL)
	if err != nil {
		s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
			Content: "❌ Invalid Playlist link!",
		})
		return nil
	}
	initialMsg, err := s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
		Content: fmt.Sprintf("Queuing %d song(s) from the playlist...", len(videoIDs)),
	})

	vc, err := connectUserVoiceChannel(s, i.GuildID, i.Member.User.ID)
	if err != nil {
		return nil
	}

	concurrencyLimit := viper.GetInt("youtube.concurrency")
	filenames, successCount := playlist.DownloadVideosConcurrently(videoIDs, ytManager, concurrencyLimit)

	if successCount == 0 {
		failMsg := "Oops! Couldn't download any songs from the playlist. 😅"
		s.FollowupMessageEdit(i.Interaction, initialMsg.ID, &discordgo.WebhookEdit{
			Content: &failMsg,
		})
		return nil
	}

	for _, filename := range filenames {
		queue.Enqueue(i.GuildID, filename, i.Member.User.Username)
	}

	gq, _ := queue.GetGuildQueue(i.GuildID)

	if gq.Session.VC == nil {
		go queue.PlayNext(s, i.GuildID, vc)
	}

	// Update user with final status by editing the initial message
	var finalContent string
	if successCount < len(videoIDs) {
		failedCount := len(videoIDs) - successCount
		if failedCount == 1 {
			finalContent = fmt.Sprintf("✅ Added %d songs to queue! (1 song couldn't be played)", successCount)
		} else {
			finalContent = fmt.Sprintf("✅ Added %d songs to queue! (%d songs couldn't be played)", successCount, failedCount)
		}
	} else {
		if successCount == 1 {
			finalContent = "🎵 Song added to queue!"
		} else {
			finalContent = fmt.Sprintf("🎵 All `%d` songs added to queue!", successCount)
		}
	}

	s.FollowupMessageEdit(i.Interaction, initialMsg.ID, &discordgo.WebhookEdit{
		Content: &finalContent,
	})
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

	queue.ClearCurrentSong(i.GuildID)

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

// currentSong displays the current song being played as well as the rest of the queue
func currentSong(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate) *interactionError {
	// Check if user is in a voice channel and bot is not in a different one
	if !checkUserVoiceChannel(s, i) {
		return nil
	}

	gq, ok := queue.GetGuildQueue(i.GuildID)
	if !ok || gq.Session.VC == nil || gq.CurrentSong == nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "🎶 Nothing is playing right now 😶"},
		})
		return nil
	}

	currentSong := gq.CurrentSong
	status := "▶️ Playing"
	if gq.Session.IsPaused() {
		status = "⏸️ Paused"
	}
	ytManager := yt.NewYouTubeManager(redis_client.RDB)

	currentID := utils.GetAudioID(currentSong.Filename)
	currentVideo, err := ytManager.GetVideoMetadata(currentID)
	if err != nil {
		sendFetchErrorResponse(s, i)
		return nil
	}

	thumbnailURL := ""
	if len(currentVideo.Thumbnails) > 0 {
		thumbnailURL = currentVideo.Thumbnails[0].URL
	}
	videoURL := fmt.Sprintf("https://www.youtube.com/watch?v=%s", currentID)

	embed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("🎵 Now Playing: %s", currentVideo.Title),
		URL:         videoURL,
		Description: fmt.Sprintf("Requested by: %s\nStatus: %s", currentSong.RequestedBy, status),
		Thumbnail:   &discordgo.MessageEmbedThumbnail{URL: thumbnailURL},
		Color:       viper.GetInt("theme"),
	}

	if len(gq.Songs) > 0 {
		queueText := "**Up Next:**\n"
		queueLimit := len(gq.Songs)
		if queueLimit > 5 {
			queueLimit = 5
		}
		for idx, item := range gq.Songs[:queueLimit] {
			itemID := utils.GetAudioID(item.Filename)
			video, err := ytManager.GetVideoMetadata(itemID)
			if err != nil {
				sendFetchErrorResponse(s, i)
				return nil
			}
			queueText += fmt.Sprintf("%d. `%s` (requested by %s)\n", idx+1, video.Title, item.RequestedBy)
		}
		if len(gq.Songs) > 5 {
			queueText += fmt.Sprintf("...and %d more", len(gq.Songs)-5)
		}
		looped := "🔁"
		if !gq.Loop {
			looped = ""
		}
		embed.Fields = []*discordgo.MessageEmbedField{
			{
				Name:  fmt.Sprintf("Queue %s", looped),
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
	if !ok || gq.Session.VC == nil || gq.CurrentSong == nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "🎶 The queue is empty 😶"},
		})
		return nil
	}

	guild, _ := s.Guild(i.GuildID)
	embed := &discordgo.MessageEmbed{
		Title: fmt.Sprintf("🎶 Queue for `%s`", guild.Name),
		Color: viper.GetInt("theme"),
	}
	ytManager := yt.NewYouTubeManager(redis_client.RDB)

	queueText := ""
	currentID := utils.GetAudioID(gq.CurrentSong.Filename)
	currentVideo, err := ytManager.GetVideoMetadata(currentID)
	if err != nil {
		sendFetchErrorResponse(s, i)
		return nil
	}
	queueText += fmt.Sprintf("1. `%s` (requested by %s) ▶️\n", currentVideo.Title, gq.CurrentSong.RequestedBy)

	for idx, item := range gq.Songs {
		itemID := utils.GetAudioID(item.Filename)
		video, err := ytManager.GetVideoMetadata(itemID)
		if err != nil {
			sendFetchErrorResponse(s, i)
			return nil
		}
		queueText += fmt.Sprintf("%d. `%s` (requested by %s)\n", idx+2, video.Title, item.RequestedBy)
	}
	looped := "🔁"
	if !gq.Loop {
		looped = ""
	}
	embed.Fields = []*discordgo.MessageEmbedField{
		{
			Name:  fmt.Sprintf("Queue %s", looped),
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
	if !ok || gq.Session.VC == nil || len(gq.Songs) == 0 {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "🎶 The queue is empty 😶"},
		})
		return nil
	}

	if err := queue.ShuffleGuildQueue(i.GuildID); err != nil {
		return &interactionError{err: err, message: "Failed to shuffle queue"}
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Content: "🔀 Queue shuffled!"},
	})
	return nil
}

// loopQueue toggles the loop the current song queue
func loopQueue(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate) *interactionError {
	// Check if user is in a voice channel and bot is not in a different one
	if !checkUserVoiceChannel(s, i) {
		return nil
	}
	gq, ok := queue.GetGuildQueue(i.GuildID)
	if !ok || gq.Session.VC == nil || gq.CurrentSong == nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "🎶 The queue is empty 😶"},
		})
		return nil
	}
	looped, err := queue.LoopGuildQueue(i.GuildID)
	if err != nil {
		return &interactionError{err: err, message: "Failed to toggle loop"}
	}

	status := "enabled"
	if !looped {
		status = "disabled"
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("🔁 Loop %s", status),
		},
	})
	return nil
}

// clearQueue clears the curreng song queue
func clearQueue(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate) *interactionError {
	// Check if user is in a voice channel and bot is not in a different one
	if !checkUserVoiceChannel(s, i) {
		return nil
	}

	gq, ok := queue.GetGuildQueue(i.GuildID)
	if !ok || gq.Session.VC == nil || gq.CurrentSong == nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "🎶 The queue is empty 😶"},
		})
		return nil
	}

	// Clear the queue and stop current song
	queue.ClearGuildQueue(i.GuildID)

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Content: "🗑️ Queue cleared!"},
	})

	return nil
}
