package app

import (
 "context"
 "ta/internal/interactive"
 "strings"
 "testing"
 "ta/internal/character"
)

func TestGroupContextDoesNotCopyRawNarration(t *testing.T) {
 app,storyID:=characterChatMattersApp(t,answerFor(`{"reply":"你好。"}`))
 c,err:=app.interactive.StoryContext(storyID,"main");if err!=nil {t.Fatal(err)}
 c.Snapshot.CurrentTurn.Narrative="GROUP_UNWITNESSED_SECRET"
 got:=groupSceneContext(c,character.Scene{WorldTime:"午后",Location:"COMMON_LOCATION"})
 if strings.Contains(got,"GROUP_UNWITNESSED_SECRET") {t.Fatal("raw narration entered shared group context")}
 if !strings.Contains(got,"COMMON_LOCATION") {t.Fatal("common physical location was removed")}
}

func TestGroupModelReceivesKnownFactsInsteadOfNarration(t *testing.T) {
 prompts:=make(chan string,1)
 app,storyID:=characterChatMattersApp(t,func(prompt string)string {prompts<-prompt;return `{"lines":[{"author_id":"陈婆婆","text":"我记得。"}]}`})
 _,err:=app.interactive.AppendCharacterFacts(storyID,interactive.AppendCharacterFactsRequest{BranchID:"main",WorldTime:"午后",Facts:character.TurnCharacterFacts{Knowledge:[]character.KnowledgeDraft{{CharacterID:"陈婆婆",Statement:"GROUP_CONFIRMED_MEMORY",Nature:character.NatureExperienced,Audience:character.AudienceSingle}}}})
 if err!=nil {t.Fatal(err)}
 group,err:=app.CreateGroup(GroupCreateRequest{StoryID:storyID,BranchID:"main",Name:"At the ferry",MemberIDs:[]string{"陈婆婆"}})
 if err!=nil {t.Fatal(err)}
 result,err:=app.SendGroupMessage(context.Background(),GroupSendRequest{StoryID:storyID,BranchID:"main",GroupID:group.ID,Message:"你记得什么？",CommandID:"group-boundary-once"})
 if err!=nil {t.Fatal(err)}
 prompt:=<-prompts
 if strings.Contains(prompt,"北岸东码头第三间货棚") {t.Fatal("raw latest narrative reached group model")}
 if !strings.Contains(prompt,"GROUP_CONFIRMED_MEMORY") {t.Fatal("committed common knowledge did not reach group model")}
 if len(result.Lines)==0 {t.Fatal("group produced no committed lines")}
 for _,line:=range result.Lines {if line.Speaker==character.SpeakerCharacter && line.AuthorID!="陈婆婆" {t.Fatalf("speaker identity changed: %+v",line)}}
}
