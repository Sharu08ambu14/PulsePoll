package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/mail"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/golang-jwt/jwt/v5"
	"github.com/joho/godotenv"
	"golang.org/x/crypto/bcrypt"

	"pulsepoll/internal/store"
)

type App struct {
	store     *store.MongoStore
	jwtSecret []byte
	redis     *redis.Client
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type signupRequest struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type createPollRequest struct {
	Question string   `json:"question"`
	Options  []string `json:"options"`
}

type voteRequest struct {
	OptionID string `json:"optionId"`
	VoterKey string `json:"voterKey"`
}

type authUser struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

func main() {
	startupStarted := time.Now()
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Fatal("load .env: ", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	mongoURI := strings.TrimSpace(os.Getenv("MONGO_URI"))
	if mongoURI == "" {
		log.Fatal("MONGO_URI is required")
	}
	database := strings.TrimSpace(os.Getenv("MONGO_DATABASE"))
	if database == "" {
		database = "pulsepoll"
	}
	log.Print("Connecting to MongoDB...")
	mongoStore, err := store.ConnectMongo(ctx, mongoURI, database)
	if err != nil {
		log.Fatal("connect to MongoDB: ", err)
	}
	log.Printf("MongoDB connected after %s", time.Since(startupStarted).Round(time.Millisecond))
	defer mongoStore.Close(context.Background())

	redisURL := strings.TrimSpace(os.Getenv("REDIS_URL"))
	if redisURL == "" {
		log.Fatal("REDIS_URL is required")
	}
	redisOptions, err := redis.ParseURL(redisURL)
	if err != nil {
		log.Fatal("parse REDIS_URL: ", err)
	}
	log.Print("Connecting to Redis...")
	redisClient := redis.NewClient(redisOptions)
	if err := redisClient.Ping(ctx).Err(); err != nil {
		_ = redisClient.Close()
		log.Fatal("connect to Redis: ", err)
	}
	defer redisClient.Close()
	log.Printf("MongoDB and Redis are ready after %s", time.Since(startupStarted).Round(time.Millisecond))

	app := &App{
		store:     mongoStore,
		jwtSecret: []byte(os.Getenv("JWT_SECRET")),
		redis:     redisClient,
	}
	if len(app.jwtSecret) < 32 {
		log.Fatal("JWT_SECRET must contain at least 32 characters")
	}
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery(), corsMiddleware())
	r.GET("/health", app.handleHealth)

	api := r.Group("/api")
	api.POST("/auth/signup", app.handleSignup)
	api.POST("/auth/login", app.handleLogin)
	api.GET("/polls", app.handleListPolls)
	api.POST("/polls", app.authRequired(), app.handleCreatePoll)
	api.GET("/polls/:id", app.handleGetPoll)
	api.POST("/polls/:id/vote", app.handleVote)
	api.GET("/polls/:id/stream", app.handleStream)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("Starting PulsePoll API on port %s", port)
	if err := r.Run("0.0.0.0:" + port); err != nil {
		log.Fatal(err)
	}
}

func corsMiddleware() gin.HandlerFunc {
	allowedOrigin := os.Getenv("FRONTEND_ORIGIN")
	if allowedOrigin == "" {
		allowedOrigin = "http://localhost:5173"
	}

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin == allowedOrigin {
			c.Header("Access-Control-Allow-Origin", allowedOrigin)
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type")
			c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func (a *App) handleHealth(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	if err := a.redis.Ping(ctx).Err(); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unhealthy", "dependency": "redis"})
		return
	}
	if err := a.store.Ping(ctx); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unhealthy", "dependency": "mongodb"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (a *App) handleSignup(c *gin.Context) {
	var req signupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	name := strings.TrimSpace(req.Name)
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if len(name) < 2 || len(name) > 80 || len(req.Password) < 8 || len(req.Password) > 72 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name must be 2-80 characters and password 8-72 characters"})
		return
	}
	if address, err := mail.ParseAddress(email); err != nil || address.Address != email {
		c.JSON(http.StatusBadRequest, gin.H{"error": "valid email is required"})
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not secure password"})
		return
	}
	userID, err := newID("user")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create account"})
		return
	}
	user := store.User{
		ID:        userID,
		Name:      name,
		Email:     email,
		Password:  string(hashedPassword),
		CreatedAt: time.Now().UTC(),
	}
	if err := a.store.CreateUser(c.Request.Context(), user); err != nil {
		if errors.Is(err, store.ErrEmailTaken) {
			c.JSON(http.StatusConflict, gin.H{"error": "email already registered"})
			return
		}
		log.Printf("create user: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create account"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "account created",
		"user":    authUser{ID: user.ID, Name: user.Name, Email: user.Email},
	})
}

func (a *App) handleLogin(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	user, err := a.store.FindUserByEmail(c.Request.Context(), email)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)) != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	token, err := generateToken(user.ID, a.jwtSecret)
	if err != nil {
		log.Printf("generate auth token: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not log in"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"token": token,
		"user":  authUser{ID: user.ID, Name: user.Name, Email: user.Email},
	})
}

func (a *App) handleListPolls(c *gin.Context) {
	polls, err := a.store.ListPolls(c.Request.Context())
	if err != nil {
		log.Printf("list polls: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not load polls"})
		return
	}
	for i := range polls {
		if err := a.applyRedisCounts(c.Request.Context(), &polls[i]); err != nil {
			log.Printf("load poll counts for %s: %v", polls[i].ID, err)
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "could not load live poll counts"})
			return
		}
	}
	c.JSON(http.StatusOK, polls)
}

func (a *App) handleCreatePoll(c *gin.Context) {
	user, ok := c.Get("user")
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}
	owner, ok := user.(authUser)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid authenticated user"})
		return
	}

	var req createPollRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	question := strings.TrimSpace(req.Question)
	if question == "" || len(question) > 200 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "question must be 1-200 characters"})
		return
	}
	if len(req.Options) < 2 || len(req.Options) > 8 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "polls must have 2-8 options"})
		return
	}

	pollID, err := newID("poll")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create poll"})
		return
	}
	poll := store.Poll{
		ID:        pollID,
		Question:  question,
		CreatedBy: owner.ID,
		CreatedAt: time.Now().UTC(),
		Active:    true,
	}
	for i, optionText := range req.Options {
		optionText = strings.TrimSpace(optionText)
		if optionText == "" || len(optionText) > 100 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "each option must be 1-100 characters"})
			return
		}
		for _, existing := range poll.Options {
			if strings.EqualFold(existing.Text, optionText) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "poll options must be unique"})
				return
			}
		}
		optionID, err := newID(fmt.Sprintf("option-%d", i+1))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create poll"})
			return
		}
		poll.Options = append(poll.Options, store.PollOption{ID: optionID, Text: optionText})
	}

	if err := a.store.CreatePoll(c.Request.Context(), poll); err != nil {
		log.Printf("create poll: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not save poll"})
		return
	}
	if err := a.initializeRedisCounts(c.Request.Context(), poll); err != nil {
		log.Printf("initialize Redis counts for %s: %v", poll.ID, err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "poll saved but live results could not be initialized"})
		return
	}

	c.JSON(http.StatusCreated, poll)
}

func (a *App) handleGetPoll(c *gin.Context) {
	poll, err := a.store.GetPoll(c.Request.Context(), c.Param("id"))
	if errors.Is(err, store.ErrPollNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "poll not found"})
		return
	}
	if err != nil {
		log.Printf("get poll: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not load poll"})
		return
	}
	if err := a.applyRedisCounts(c.Request.Context(), &poll); err != nil {
		log.Printf("load Redis counts for %s: %v", poll.ID, err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "could not load live poll counts"})
		return
	}
	c.JSON(http.StatusOK, poll)
}

func (a *App) handleVote(c *gin.Context) {
	pollID := c.Param("id")
	var req voteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	voterKey := strings.TrimSpace(req.VoterKey)
	if req.OptionID == "" || len(voterKey) < 16 || len(voterKey) > 128 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "optionId and a 16-128 character voterKey are required"})
		return
	}

	poll, err := a.store.GetPoll(c.Request.Context(), pollID)
	if errors.Is(err, store.ErrPollNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "poll not found"})
		return
	}
	if err != nil {
		log.Printf("load poll before vote: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not load poll"})
		return
	}
	if !poll.Active {
		c.JSON(http.StatusConflict, gin.H{"error": "poll is closed"})
		return
	}
	validOption := false
	for _, option := range poll.Options {
		if option.ID == req.OptionID {
			validOption = true
			break
		}
	}
	if !validOption {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid option selected"})
		return
	}

	err = a.store.CreateVote(c.Request.Context(), store.Vote{
		PollID:    pollID,
		OptionID:  req.OptionID,
		VoterKey:  voterKey,
		CreatedAt: time.Now().UTC(),
	})
	if errors.Is(err, store.ErrAlreadyVoted) {
		c.JSON(http.StatusConflict, gin.H{"error": "already voted"})
		return
	}
	if err != nil {
		log.Printf("save vote: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not record vote"})
		return
	}

	if err := a.recordRedisVote(c.Request.Context(), pollID, req.OptionID); err != nil {
		log.Printf("update Redis vote count for %s: %v", pollID, err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "vote saved, but live results could not be updated"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"message": "vote recorded"})
}

func (a *App) handleStream(c *gin.Context) {
	pollID := c.Param("id")
	if _, err := a.store.GetPoll(c.Request.Context(), pollID); errors.Is(err, store.ErrPollNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "poll not found"})
		return
	} else if err != nil {
		log.Printf("verify poll for stream: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not load poll"})
		return
	}

	pubsub := a.redis.Subscribe(c.Request.Context(), pollChannel(pollID))
	defer pubsub.Close()
	if _, err := pubsub.Receive(c.Request.Context()); err != nil {
		log.Printf("subscribe to poll %s: %v", pollID, err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "could not subscribe to live results"})
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)
	c.Writer.Flush()

	messages := pubsub.Channel()
	keepAlive := time.NewTicker(20 * time.Second)
	defer keepAlive.Stop()
	for {
		select {
		case <-c.Request.Context().Done():
			return
		case <-keepAlive.C:
			if _, err := c.Writer.WriteString(": keep-alive\n\n"); err != nil {
				return
			}
			c.Writer.Flush()
		case message, ok := <-messages:
			if !ok {
				return
			}
			if _, err := fmt.Fprintf(c.Writer, "event: poll-update\ndata: %s\n\n", message.Payload); err != nil {
				return
			}
			c.Writer.Flush()
		}
	}
}

func (a *App) authRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		parts := strings.Fields(c.GetHeader("Authorization"))
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "valid bearer token required"})
			return
		}
		token, err := jwt.Parse(parts[1], func(token *jwt.Token) (interface{}, error) {
			if token.Method != jwt.SigningMethodHS256 {
				return nil, errors.New("unexpected signing method")
			}
			return a.jwtSecret, nil
		}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
		if err != nil || !token.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}
		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token claims"})
			return
		}
		userID, ok := claims["sub"].(string)
		if !ok || userID == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token subject"})
			return
		}

		user, err := a.store.FindUserByID(c.Request.Context(), userID)
		if errors.Is(err, store.ErrUserNotFound) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "user no longer exists"})
			return
		}
		if err != nil {
			log.Printf("load authenticated user: %v", err)
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "could not verify user"})
			return
		}
		c.Set("user", authUser{ID: user.ID, Name: user.Name, Email: user.Email})
		c.Next()
	}
}

func generateToken(userID string, secret []byte) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": userID,
		"exp": time.Now().Add(12 * time.Hour).Unix(),
	})
	return token.SignedString(secret)
}

func (a *App) initializeRedisCounts(ctx context.Context, poll store.Poll) error {
	counts, err := a.store.CountVotesByOption(ctx, poll.ID)
	if err != nil {
		return err
	}
	values := make(map[string]interface{}, len(poll.Options))
	for _, option := range poll.Options {
		values[option.ID] = counts[option.ID]
	}
	_, err = a.redis.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.Del(ctx, pollCountsKey(poll.ID))
		pipe.HSet(ctx, pollCountsKey(poll.ID), values)
		return nil
	})
	return err
}

func (a *App) applyRedisCounts(ctx context.Context, poll *store.Poll) error {
	values, err := a.redis.HGetAll(ctx, pollCountsKey(poll.ID)).Result()
	if err != nil {
		return err
	}
	validCounts := len(values) == len(poll.Options)
	if validCounts {
		for _, option := range poll.Options {
			count, exists := values[option.ID]
			parsedCount, parseErr := strconv.Atoi(count)
			if !exists || parseErr != nil || parsedCount < 0 {
				validCounts = false
				break
			}
		}
	}
	if !validCounts {
		if err := a.initializeRedisCounts(ctx, *poll); err != nil {
			return err
		}
		values, err = a.redis.HGetAll(ctx, pollCountsKey(poll.ID)).Result()
		if err != nil {
			return err
		}
	}
	for i := range poll.Options {
		count, err := strconv.Atoi(values[poll.Options[i].ID])
		if err != nil {
			return err
		}
		poll.Options[i].Votes = count
	}
	return nil
}

func (a *App) recordRedisVote(ctx context.Context, pollID, optionID string) error {
	payload, err := json.Marshal(map[string]string{
		"pollId":    pollID,
		"optionId":  optionID,
		"updatedAt": time.Now().UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		return err
	}
	_, err = a.redis.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.HIncrBy(ctx, pollCountsKey(pollID), optionID, 1)
		pipe.Publish(ctx, pollChannel(pollID), payload)
		return nil
	})
	if err != nil {
		_ = a.redis.Del(ctx, pollCountsKey(pollID)).Err()
		return err
	}
	return nil
}

func pollCountsKey(pollID string) string {
	return "pulsepoll:poll:" + pollID + ":counts"
}

func pollChannel(pollID string) string {
	return "pulsepoll:poll:" + pollID + ":updates"
}

func newID(prefix string) (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return prefix + "-" + hex.EncodeToString(value), nil
}
