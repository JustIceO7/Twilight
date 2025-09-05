package yt

import "time"

type Song struct {
	ID          string
	Title       string
	Description string
	Author      string
	Views       int
	Duration    time.Duration
	PublishDate time.Time
	URL         string
}

type Playlist struct {
	Songs []*Song
}

type User struct {
	UserID   int
	Username string
	PlayList *Playlist
}

type Server struct {
	ID          string
	Users       map[string]*User
	SharedQueue *Playlist
}
