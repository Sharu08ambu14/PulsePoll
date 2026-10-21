package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"
)

func TestMongoStorePersistsPollsAndVotes(t *testing.T) {
	uri := os.Getenv("MONGO_TEST_URI")
	if uri == "" {
		t.Skip("set MONGO_TEST_URI to run the MongoDB integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	database := fmt.Sprintf("pulsepoll_test_%d", time.Now().UnixNano())
	repository, err := ConnectMongo(ctx, uri, database)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = repository.client.Database(database).Drop(context.Background())
		_ = repository.Close(context.Background())
	})

	user := User{
		ID:        "user-test",
		Name:      "Test User",
		Email:     "test@example.com",
		Password:  "bcrypt-hash",
		CreatedAt: time.Now().UTC(),
	}
	if err := repository.CreateUser(ctx, user); err != nil {
		t.Fatal(err)
	}
	if err := repository.CreateUser(ctx, user); !errors.Is(err, ErrEmailTaken) {
		t.Fatalf("expected duplicate email error, got %v", err)
	}
	foundUser, err := repository.FindUserByEmail(ctx, user.Email)
	if err != nil || foundUser.ID != user.ID {
		t.Fatalf("find user = (%+v, %v), want ID %q", foundUser, err, user.ID)
	}

	poll := Poll{
		ID:        "poll-test",
		Question:  "Choose",
		CreatedBy: user.ID,
		CreatedAt: time.Now().UTC(),
		Active:    true,
		Options: []PollOption{
			{ID: "option-a", Text: "A"},
			{ID: "option-b", Text: "B"},
		},
	}
	if err := repository.CreatePoll(ctx, poll); err != nil {
		t.Fatal(err)
	}
	foundPoll, err := repository.GetPoll(ctx, poll.ID)
	if err != nil || foundPoll.Question != poll.Question || len(foundPoll.Options) != 2 {
		t.Fatalf("get poll = (%+v, %v)", foundPoll, err)
	}

	vote := Vote{PollID: poll.ID, OptionID: "option-a", VoterKey: "browser-voter-key-1", CreatedAt: time.Now().UTC()}
	if err := repository.CreateVote(ctx, vote); err != nil {
		t.Fatal(err)
	}
	if err := repository.CreateVote(ctx, vote); !errors.Is(err, ErrAlreadyVoted) {
		t.Fatalf("expected duplicate voter error, got %v", err)
	}
	counts, err := repository.CountVotesByOption(ctx, poll.ID)
	if err != nil {
		t.Fatal(err)
	}
	if counts["option-a"] != 1 || counts["option-b"] != 0 {
		t.Fatalf("vote counts = %v, want option-a=1", counts)
	}
}
