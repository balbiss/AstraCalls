package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// Cache da mídia já decodificada do disparo (InoovaCall). Quem dispara 1 número
// por requisição (pra controlar intervalo aleatório entre ligações) faria o
// AstraCalls baixar + decodificar a MESMA mídia a cada ligação — no vídeo isso é
// um encode H264 de 2 passadas por chamada. Aqui guardamos o PCM/frames por
// (audio_url + opções de vídeo) por um tempo curto. Só vale para audio_url:
// audio_base64 vem inline e não tem identidade estável.

const (
	bcCacheTTL = 30 * time.Minute
	bcCacheMax = 16
)

type bcCacheEntry struct {
	pcm     []float32
	frames  []vframe
	expires time.Time
}

var bcCache = struct {
	sync.Mutex
	m map[string]*bcCacheEntry
}{m: map[string]*bcCacheEntry{}}

func bcCacheKey(b broadcastRequest) string {
	if b.AudioURL == "" || b.AudioBase64 != "" {
		return ""
	}
	raw := b.AudioURL
	if b.Video {
		o := b.videoOpts().normalize()
		raw += fmt.Sprintf("|v|%+v", o)
	}
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func bcCacheGet(key string) ([]float32, []vframe, bool) {
	if key == "" {
		return nil, nil, false
	}
	bcCache.Lock()
	defer bcCache.Unlock()
	e, ok := bcCache.m[key]
	if !ok || time.Now().After(e.expires) {
		delete(bcCache.m, key)
		return nil, nil, false
	}
	e.expires = time.Now().Add(bcCacheTTL) // uso renova
	return e.pcm, e.frames, true
}

func bcCachePut(key string, pcm []float32, frames []vframe) {
	if key == "" {
		return
	}
	bcCache.Lock()
	defer bcCache.Unlock()
	now := time.Now()
	for k, e := range bcCache.m {
		if now.After(e.expires) {
			delete(bcCache.m, k)
		}
	}
	if len(bcCache.m) >= bcCacheMax {
		var oldestK string
		var oldest time.Time
		for k, e := range bcCache.m {
			if oldestK == "" || e.expires.Before(oldest) {
				oldestK, oldest = k, e.expires
			}
		}
		delete(bcCache.m, oldestK)
	}
	bcCache.m[key] = &bcCacheEntry{pcm: pcm, frames: frames, expires: now.Add(bcCacheTTL)}
}
