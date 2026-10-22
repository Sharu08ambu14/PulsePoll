package store

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	ErrUserNotFound = errors.New("user not found")
	ErrPollNotFound = errors.New("poll not found")
	ErrEmailTaken   = errors.New("email already registered")
	ErrAlreadyVoted = errors.New("user already voted")
)

type User struct {
	ID        string    `json:"id" bson:"_id"`
	Name      string    `json:"name" bson:"name"`
	Email     string    `json:"email" bson:"email"`
	Password  string    `json:"-" bson:"password"`
	CreatedAt time.Time `json:"createdAt" bson:"created_at"`
}

type PollOption struct {
	ID    string `json:"id" bson:"id"`
	Text  string `json:"text" bson:"text"`
	Votes int    `json:"votes" bson:"-"`
}

type Poll struct {
	ID        string       `json:"id" bson:"_id"`
	Question  string       `json:"question" bson:"question"`
	Options   []PollOption `json:"options" bson:"options"`
	CreatedBy string       `json:"createdBy" bson:"created_by"`
	CreatedAt time.Time    `json:"createdAt" bson:"created_at"`
	Active    bool         `json:"active" bson:"active"`
}

type Vote struct {
	PollID    string    `bson:"poll_id"`
	OptionID  string    `bson:"option_id"`
	VoterKey  string    `bson:"voter_key"`
	CreatedAt time.Time `bson:"created_at"`
}

type MongoStore struct {
	client *mongo.Client
	users  *mongo.Collection
	polls  *mongo.Collection
	votes  *mongo.Collection
}

func ConnectMongo(ctx context.Context, uri, database string) (*MongoStore, error) {
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return nil, err
	}
	if err := client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, err
	}

	db := client.Database(database)
	store := &MongoStore{
		client: client,
		users:  db.Collection("users"),
		polls:  db.Collection("polls"),
		votes:  db.Collection("votes"),
	}
	if err := store.createIndexes(ctx); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, err
	}
	return store, nil
}

func (s *MongoStore) Close(ctx context.Context) error {
	return s.client.Disconnect(ctx)
}

func (s *MongoStore) Ping(ctx context.Context) error {
	return s.client.Ping(ctx, nil)
}

func (s *MongoStore) createIndexes(ctx context.Context) error {
	_, err := s.users.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "email", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return err
	}

	_, err = s.votes.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "poll_id", Value: 1}, {Key: "voter_key", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return err
	}

	_, err = s.votes.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "poll_id", Value: 1}, {Key: "option_id", Value: 1}},
	})
	return err
}

func (s *MongoStore) CreateUser(ctx context.Context, user User) error {
	_, err := s.users.InsertOne(ctx, user)
	if mongo.IsDuplicateKeyError(err) {
		return ErrEmailTaken
	}
	return err
}

func (s *MongoStore) FindUserByEmail(ctx context.Context, email string) (User, error) {
	var user User
	err := s.users.FindOne(ctx, bson.M{"email": email}).Decode(&user)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return User{}, ErrUserNotFound
	}
	return user, err
}

func (s *MongoStore) FindUserByID(ctx context.Context, id string) (User, error) {
	var user User
	err := s.users.FindOne(ctx, bson.M{"_id": id}).Decode(&user)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return User{}, ErrUserNotFound
	}
	return user, err
}

func (s *MongoStore) ListPolls(ctx context.Context) ([]Poll, error) {
	cursor, err := s.polls.Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	polls := make([]Poll, 0)
	if err := cursor.All(ctx, &polls); err != nil {
		return nil, err
	}
	return polls, nil
}

func (s *MongoStore) ListPollsByOwner(ctx context.Context, ownerID string) ([]Poll, error) {
	cursor, err := s.polls.Find(ctx, bson.M{"created_by": ownerID}, options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	polls := make([]Poll, 0)
	if err := cursor.All(ctx, &polls); err != nil {
		return nil, err
	}
	return polls, nil
}

func (s *MongoStore) GetPoll(ctx context.Context, id string) (Poll, error) {
	var poll Poll
	err := s.polls.FindOne(ctx, bson.M{"_id": id}).Decode(&poll)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return Poll{}, ErrPollNotFound
	}
	return poll, err
}

func (s *MongoStore) CreatePoll(ctx context.Context, poll Poll) error {
	_, err := s.polls.InsertOne(ctx, poll)
	return err
}

func (s *MongoStore) CreateVote(ctx context.Context, vote Vote) error {
	_, err := s.votes.InsertOne(ctx, vote)
	if mongo.IsDuplicateKeyError(err) {
		return ErrAlreadyVoted
	}
	return err
}

func (s *MongoStore) CountVotesByOption(ctx context.Context, pollID string) (map[string]int, error) {
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"poll_id": pollID}}},
		{{Key: "$group", Value: bson.M{"_id": "$option_id", "count": bson.M{"$sum": 1}}}},
	}
	cursor, err := s.votes.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	counts := make(map[string]int)
	for cursor.Next(ctx) {
		var result struct {
			OptionID string `bson:"_id"`
			Count    int    `bson:"count"`
		}
		if err := cursor.Decode(&result); err != nil {
			return nil, err
		}
		counts[result.OptionID] = result.Count
	}
	if err := cursor.Err(); err != nil {
		return nil, err
	}
	return counts, nil
}
