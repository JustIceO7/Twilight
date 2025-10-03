# Twilight (Music Discord Bot)
A Discord bot written in Golang (v1.25.0+) that streams audio concurrently, manages song queues, and supports YouTube playback, playlists, and multiple guilds.  
  
Features intelligent audio caching with Redis-based TTL management and automatic cleanup of unused files.

## Architecture
- **PostgreSQL**: Persistent storage for user playlists.
- **Redis**: In-memory caching for song data.
- **Docker**: Containerized deployment with docker-compose for easy setup and management. All core dependencies (FFmpeg, yt-dlp, PostgreSQL, Redis) are pre-configured within the Docker containers.

## Docker Ports
- **8080**: Bot application.
- **6379**: Redis server.
- **5432**: PostgreSQL database server.

## Setup
1. Clone this repository:
```
git clone https://github.com/JustIceO7/Twilight.git
```

2. Create a .env file in the root directory with:
```
discord_token=your_discord_token
discord_app_id=your_app_id
prefix=bot_prefix
theme=bot_theme_hexcode
```

3. Add the bot to your Discord server with permissions to connect to voice channels, post messages, react and speak.
   
4. Ensure ***`MESSAGE CONTENT INTENT`*** is `ON` within Discord Developer Application.
<img width="1408" height="136" alt="image" src="https://github.com/user-attachments/assets/685cd65b-ff38-466e-83b4-b12834abfa2e" />

5. Install Docker Desktop (Windows/macOS) or Docker Engine (Linux).
  
6. Run the application:
```
docker-compose up
```

## Commands
`^help` – Shows all available commands.

### Music Controls
`/play <url>` - Play a song from a YouTube URL.  
`/pause` - Pause the current song.  
`/resume` - Resume the paused song.  
`/skip` - Skip the current song.  
`/shuffle` - Shuffle the current song queue.  
`/queue` - Show the current song queue.  
`/np` - Show the song that's now playing.  
`/sinfo` - Show the song info from a YouTube URL.  
`/loop` - Toggle loop for the current song queue.  
`/disconnect` - Stop playback and disconnect the bot from the voice channel.  
`/leave` - Stop playback and disconnect the bot from the voice channel.

### Playlist Management
`/playlist view` - View your playlist.  
`/playlist add <song>` - Add a song to your playlist (YouTube video ID).  
`/playlist remove <song>` - Remove a song from your playlist (YouTube video ID).  
`/playlist clear` - Clear your playlist.  
`/playlist play [song]` - Play a song from your playlist or the entire playlist (optional YouTube video ID).