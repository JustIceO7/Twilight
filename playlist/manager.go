package playlist

import (
	"context"

	"github.com/bwmarrin/discordgo"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type PlaylistManager struct {
	Session *discordgo.Session
	Redis   *redis.Client
	Ctx     context.Context
	DB      *pgxpool.Pool
}

func (pm *PlaylistManager) ShowPlaylist(i *discordgo.InteractionCreate) {
}

func (pm *PlaylistManager) AddSong(i *discordgo.InteractionCreate, value string) {
}

func (pm *PlaylistManager) RemoveSong(i *discordgo.InteractionCreate, value string) {
}

func (pm *PlaylistManager) ClearPlaylist(i *discordgo.InteractionCreate) {
}

func (pm *PlaylistManager) PlaySong(i *discordgo.InteractionCreate, value string) {
}

// Newmanager returns a new instance of PlayListManager
func NewManager(s *discordgo.Session, r *redis.Client, db *pgxpool.Pool, ctx context.Context) *PlaylistManager {
	return &PlaylistManager{
		Session: s,
		Redis:   r,
		DB:      db,
		Ctx:     ctx,
	}
}
