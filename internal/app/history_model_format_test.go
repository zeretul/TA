package app

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
)

func TestHistoryRequestsProviderJSONObjectMode(t *testing.T) {
	var calls atomic.Int32
	service, projectID, state := historyServiceFixture(t, func(req historyModelRequest) (string, int) {
		calls.Add(1)
		if req.ResponseFormat.Type != "json_object" {
			return "臣遵旨，国库可用银1000000两。", 200
		}
		return `{"reply":"国库可用银1000000两，粮食630000石；本次问话不动用钱粮。"}`, 200
	})
	result, err := service.Chat(context.Background(), projectID, state.ID, HistoryChatRequest{HistoryCommandRequest: HistoryCommandRequest{CommandID: "format-mode", ExpectedRevision: state.Revision}, MinisterIDs: []string{"bi-ziyan"}, Message: "只按当前账目回答钱粮。"})
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 || len(result.State.Chats) != 1 || result.State.Treasury != state.Treasury || result.State.Grain != state.Grain {
		t.Fatal("structured chat changed resources or repeated generation")
	}
}

func TestHistoryRepairsMalformedReplyOnceAgainstOriginalFacts(t *testing.T) {
	var calls atomic.Int32
	var firstFacts string
	service, projectID, state := historyServiceFixture(t, func(req historyModelRequest) (string, int) {
		count := calls.Add(1)
		if count == 1 {
			firstFacts = req.Messages[1].Content
			return "普通正文而不是约定的JSON对象。", 200
		}
		if req.ResponseFormat.Type != "json_object" || req.Messages[1].Content != firstFacts || !strings.Contains(req.Messages[0].Content, "格式修复") {
			t.Error("repair changed facts or omitted structured output instruction")
		}
		return `{"reply":"国库可用银1000000两。"}`, 200
	})
	req := HistoryChatRequest{HistoryCommandRequest: HistoryCommandRequest{CommandID: "format-repair", ExpectedRevision: state.Revision}, MinisterIDs: []string{"bi-ziyan"}, Message: "当前国库多少？"}
	result, err := service.Chat(context.Background(), projectID, state.ID, req)
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 || len(result.State.Chats) != 1 || result.State.Chats[0].Replies[0].Text != "国库可用银1000000两。" {
		t.Fatalf("reply repair failed or saved malformed output: %+v", result)
	}
	duplicate, err := service.Chat(context.Background(), projectID, state.ID, req)
	if err != nil || !duplicate.Duplicate || calls.Load() != 2 {
		t.Fatal("repaired reply retry repeated generation")
	}
}

func TestHistoryRepeatedBadFormatCannotCommitHalfQuarter(t *testing.T) {
	var calls atomic.Int32
	service, projectID, state := historyServiceFixture(t, func(historyModelRequest) (string, int) {
		if calls.Add(1) == 1 {
			return `{"adjustments":[],"events":[]}`, 200
		}
		return "not a JSON report", 200
	})
	_, err := service.Advance(context.Background(), projectID, state.ID, HistoryCommandRequest{CommandID: "bad-quarter-format", ExpectedRevision: state.Revision})
	if !errors.Is(err, ErrHistoryModelOutput) || calls.Load() != 3 {
		t.Fatalf("format correction unbounded or not attempted once: calls=%d err=%v", calls.Load(), err)
	}
	after, err := service.Read(context.Background(), projectID, state.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != state.Revision || after.Treasury != state.Treasury || after.Turn != state.Turn || len(after.Reports) != 0 {
		t.Fatal("malformed quarter explanation partially committed")
	}
}
