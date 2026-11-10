package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"ta/internal/history"
	"testing"
)

func TestHistorySourcePagesPreserveExactLargeRecords(t *testing.T) {
	large := strings.Repeat("完整史料", 6000)
	payload := []byte(`{"world":{"entities":[{"id":"long-record","text":` + fmt.Sprintf("%q", large) + `,"balance":9007199254740993}],"archived_entities":[{"id":"retired","text":"old fact"}]}}`)
	pages, err := historySourcePages(payload)
	if err != nil {
		t.Fatal(err)
	}
	var reconstructed strings.Builder
	archived, exact := false, false
	for _, p := range pages {
		b, _ := json.Marshal(p)
		if len(b) > historyPageBytes+2048 {
			t.Fatal("Unbounded page")
		}
		exact = exact || bytes.Contains(b, []byte("9007199254740993"))
		for _, e := range p.Entries {
			if e.Path == "$.world.entities[0].text" {
				if e.RecordID != "long-record" {
					t.Fatal("Split record lost lookup identity")
				}
				reconstructed.WriteString(e.Value.(string))
			}
			if e.RecordID == "retired" {
				archived = p.Archived
			}
		}
	}
	if reconstructed.String() != large || !exact || !archived {
		t.Fatal("Lossy pages")
	}
}

func TestHistoryPagedReaderRetrievesArchivedOriginal(t *testing.T) {
	var lookupDone, readDone bool
	var scanCount int
	service, projectID, _ := historyServiceFixture(t, func(req historyModelRequest) (string, int) {
		if len(req.Messages[1].Content) > historyModelInputLimit {
			t.Fatal("Unbounded request")
		}
		var input struct {
			Mode    string              `json:"mode"`
			Matches []int               `json:"lookup_matches"`
			Pages   []historySourcePage `json:"read_pages"`
			Source  historySourcePage   `json:"source_page"`
		}
		json.Unmarshal([]byte(req.Messages[1].Content), &input)
		if input.Mode == "scan" {
			scanCount++
			if input.Source.Archived {
				t.Fatal("Archived details unnecessarily scanned")
			}
			return `{"action":"notes","notes":"现行资料已读，旧任档案须检索。"}`, 200
		}
		if !lookupDone {
			lookupDone = true
			return `{"action":"read","lookup":"retired-target","notes":"查找旧任原始记录。"}`, 200
		}
		if !readDone {
			if len(input.Matches) == 0 {
				t.Fatal("Archive lookup missing")
			}
			readDone = true
			return fmt.Sprintf(`{"action":"read","pages":[%d],"notes":"读取匹配档案。"}`, input.Matches[0]), 200
		}
		if !strings.Contains(req.Messages[1].Content, "古井晴川") || !strings.Contains(req.Messages[1].Content, "9007199254740993") {
			t.Fatal("Original archive facts missing")
		}
		return `{"action":"answer","value":{"reply":"旧任档案原话：古井晴川。金额9007199254740993。"}}`, 200
	})
	_, cfg, _ := service.binding(projectID)
	rows := []map[string]any{}
	for i := 0; i < 180; i++ {
		rows = append(rows, map[string]any{"id": fmt.Sprintf("record-%d", i), "text": strings.Repeat("当前事实", 120)})
	}
	input := map[string]any{"world": map[string]any{"entities": rows, "archived_entities": []map[string]any{{"id": "retired-target", "text": "古井晴川", "balance": json.Number("9007199254740993")}}}, "message": "核对旧任原始记录"}
	reply, err := historyGenerateJSON[historyMinisterPayload](context.Background(), &cfg, "large-query", "minister", "读取正式事实", "zh-CN", input)
	if err != nil || !readDone || scanCount == 0 || !strings.Contains(reply.Reply, "古井晴川") {
		t.Fatalf("Reader failed: %v", err)
	}
}

func TestHistoryPagedQuarterAssemblesBeyondSingleResponse(t *testing.T) {
	results := []history.OrderResult{}
	orders := []map[string]any{}
	for i := 0; i < 300; i++ {
		id := fmt.Sprintf("order-%d", i)
		orders = append(orders, map[string]any{"id": id, "created_turn": 0})
		results = append(results, history.OrderResult{OrderID: id, Status: "completed", Progress: 100, Reason: strings.Repeat("核对完成。", 20)})
	}
	service, projectID, _ := historyServiceFixture(t, func(req historyModelRequest) (string, int) {
		var input struct {
			Mode    string `json:"mode"`
			Emitted int    `json:"emitted_chunks"`
		}
		json.Unmarshal([]byte(req.Messages[1].Content), &input)
		if input.Mode == "scan" {
			return `{"action":"notes","notes":"逐项结算全部政令。"}`, 200
		}
		if input.Emitted == 3 {
			return `{"action":"finish"}`, 200
		}
		value := map[string]any{"action": "append", "notes": fmt.Sprintf("已完成第%d段", input.Emitted+1), "value": map[string]any{"results": results[input.Emitted*100 : (input.Emitted+1)*100]}}
		b, _ := json.Marshal(value)
		return string(b), 200
	})
	_, cfg, _ := service.binding(projectID)
	p, err := historyGenerateJSON[history.WorldProposal](context.Background(), &cfg, "long-quarter", "world_simulation", "完整季度候选", "zh-CN", map[string]any{"orders": orders})
	b, _ := json.Marshal(p)
	if err != nil || len(p.Results) != 300 || len(b) <= historyModelOutputLimit {
		t.Fatalf("Chunked result failed: %v, %d", err, len(b))
	}
}

func TestHistoryPagedReaderCanRevisitSourceAfterProgress(t *testing.T) {
	calls := 0
	service, projectID, _ := historyServiceFixture(t, func(historyModelRequest) (string, int) {
		calls++
		switch calls {
		case 1, 3:
			return `{"action":"read","pages":[0]}`, 200
		case 2:
			return `{"action":"append","value":{"results":[{"order_id":"first","status":"waiting","progress":0,"reason":"尚未到期"}]}}`, 200
		default:
			return `{"action":"finish"}`, 200
		}
	})
	_, cfg, _ := service.binding(projectID)
	orders := make([]map[string]int, 129)
	for i := range orders {
		orders[i] = map[string]int{"created_turn": 0}
	}
	p, err := historyGenerateJSON[history.WorldProposal](context.Background(), &cfg, "revisit", "world_simulation", "完整季度候选", "zh-CN", map[string]any{"orders": orders})
	if err != nil || len(p.Results) != 1 || calls != 4 {
		t.Fatalf("Source revisit rejected: %v calls=%d", err, calls)
	}
}

func TestHistoryPagedReaderStopsParaphrasedRetrievalLoop(t *testing.T) {
	calls := 0
	service, projectID, _ := historyServiceFixture(t, func(req historyModelRequest) (string, int) {
		var mode struct {
			Mode string `json:"mode"`
		}
		json.Unmarshal([]byte(req.Messages[1].Content), &mode)
		if mode.Mode == "scan" {
			return `{"action":"notes","notes":"已读现行资料"}`, 200
		}
		calls++
		return fmt.Sprintf(`{"action":"read","pages":[0],"notes":"再次核对%d"}`, calls), 200
	})
	_, cfg, _ := service.binding(projectID)
	orders := make([]map[string]int, 129)
	for i := range orders {
		orders[i] = map[string]int{"created_turn": 0}
	}
	_, err := historyGenerateJSON[history.WorldProposal](context.Background(), &cfg, "loop", "world_simulation", "完整季度候选", "zh-CN", map[string]any{"orders": orders})
	if err == nil || calls != 3 {
		t.Fatalf("Unbounded retrieval loop: %v calls=%d", err, calls)
	}
}
