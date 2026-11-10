package app

import (
 "context"
 "errors"
 "strings"
 "sync/atomic"
 "testing"

 "ta/internal/character"
 "ta/internal/interactive"
)

func TestLetterWorldFaceToFaceAndRemoteBoundary(t *testing.T) {
 for _,present:=range []bool{true,false} {
  name:="remote";if present {name="present"}
  t.Run(name,func(t *testing.T){
   var calls atomic.Int32
   prompts:=make(chan string,2)
   app,_:=characterChatMattersApp(t,func(prompt string)string{ calls.Add(1);prompts<-prompt;return `{"reply":"我们就在这里谈。"}` })
   system,err:=interactive.NewActorStateLibrary(t.TempDir()).Get(interactive.ActorStateXiuxianID)
   if err!=nil {t.Fatal(err)}
   story,err:=app.interactive.CreateStory(interactive.CreateStoryRequest{Title:"Letter contact",ActorState:&system.ActorState,PackBinding:&interactive.StoryPackBinding{
    PackID:"letter-contact",Scope:interactive.PackScopeFromID("letter-contact"),Gameplay:&interactive.StoryGameplayProfile{StatusTemplate:"world-actions-v1",Growth:"none",Combat:"none",Communication:"letter",LetterDeliverySeconds:86400},
   }})
   if err!=nil {t.Fatal(err)}
   audience:=[]any{interactive.DefaultActorID};if present {audience=append(audience,"same-person")}
   _,_,err=app.interactive.AppendTurnWithState(story.ID,interactive.AppendTurnWithStateRequest{
    BranchID:"main",User:"Observe",Narrative:"A quiet afternoon.",
    ActorOps:[]interactive.ActorStateOp{{Op:"set",ActorID:interactive.DefaultStoryContextActorID,FieldID:"在场角色",Value:audience,Reason:"Committed physical presence"}},
    TurnResult:&interactive.TurnResult{Choices:[]string{"继续行动","观察环境","询问同伴","检查状态","暂时等待"}},
   })
   if err!=nil {t.Fatal(err)}
   _,err=app.interactive.AppendCharacterFacts(story.ID,interactive.AppendCharacterFactsRequest{BranchID:"main",WorldTime:"午后",Profiles:[]character.Profile{{ID:"same-person",Name:"Same person",Description:"IDENTITY_MARKER",Gender:character.GenderFemale}}})
   if err!=nil {t.Fatal(err)}
   before,err:=app.interactive.StoryContext(story.ID,"main");if err!=nil {t.Fatal(err)}
   req:=CharacterChatRequest{StoryID:story.ID,BranchID:"main",CharacterID:"same-person",Message:"Can we talk here?",CommandID:"letter-local-once"}
   result,err:=app.SendCharacterMessage(context.Background(),req)
   if !present {
    if !errors.Is(err,ErrCharacterChatUnavailable) {t.Fatalf("remote letter contact error=%v",err)}
    if calls.Load()!=0 {t.Fatal("remote letter contact called model")}
    state,e:=app.interactive.CharacterState(story.ID,"main");if e!=nil {t.Fatal(e)}
    if len(state.ExchangesFor("same-person"))!=0 {t.Fatal("rejected remote contact committed dialogue")}
    return
   }
   if err!=nil {t.Fatalf("present letter-world contact rejected: %v",err)}
   if result.CharacterID!="same-person" {t.Fatalf("reply identity changed: %+v",result)}
   if !strings.Contains(<-prompts,"IDENTITY_MARKER") {t.Fatal("model lost committed person identity")}
   state,err:=app.interactive.CharacterState(story.ID,"main");if err!=nil {t.Fatal(err)}
   exchanges:=state.ExchangesFor("same-person")
   if len(exchanges)!=2 {t.Fatalf("expected one private exchange pair: %+v",exchanges)}
   for _,x:=range exchanges {if x.CharacterID!="same-person" || x.Channel!=character.ChannelPrivate {t.Fatalf("wrong identity/channel: %+v",x)}}
   after,err:=app.interactive.StoryContext(story.ID,"main");if err!=nil {t.Fatal(err)}
   if before.Snapshot.TurnCount!=after.Snapshot.TurnCount || interactive.SnapshotClockSeconds(before.Snapshot)!=interactive.SnapshotClockSeconds(after.Snapshot) {t.Fatal("local conversation advanced main story")}
   if _,err:=app.SendCharacterMessage(context.Background(),req);err!=nil {t.Fatal(err)}
   if calls.Load()!=1 {t.Fatal("retry generated a second conversation")}
  })
 }
}
