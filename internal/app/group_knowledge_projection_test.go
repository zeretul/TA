package app

import (
 "strings"
 "testing"
 "ta/internal/character"
)

func TestGroupKnowledgeIntersectionAndRemoteScene(t *testing.T) {
 state:=character.State{Knowledge:[]character.KnowledgeRecord{
  {ID:"shared",CharacterID:"a",Statement:"SHARED_FACT",Nature:character.NatureTold,Audience:character.AudienceParticipants,Holders:[]string{"a","b"}},
  {ID:"private",CharacterID:"a",Statement:"PRIVATE_SECRET",Nature:character.NatureSecret,Audience:character.AudienceSingle},
  {ID:"old",CharacterID:"a",Statement:"SUPERSEDED_SECRET",Nature:character.NatureTold,Audience:character.AudienceParticipants,Holders:[]string{"a","b"},SupersededBy:"new"},
  {ID:"guess",CharacterID:"a",Statement:"SHARED_GUESS",Nature:character.NatureConjecture,Audience:character.AudienceParticipants,Holders:[]string{"a","b"}},
 }}
 scene:=character.Scene{WorldTime:"KNOWN_TIME",Location:"LOCAL_ROOM",Title:"SECRET_EVENT",Present:[]string{"a","b","LOCAL_BYSTANDER"}}
 group:=character.Group{Channel:character.ChannelMessage}
 remote:=groupConversationContext(state,group,[]string{"a","b"},scene)
 for _,marker:=range []string{"LOCAL_ROOM","SECRET_EVENT","LOCAL_BYSTANDER","PRIVATE_SECRET","SUPERSEDED_SECRET"} {if strings.Contains(remote,marker) {t.Fatalf("remote leak %s: %s",marker,remote)}}
 for _,marker:=range []string{"KNOWN_TIME","SHARED_FACT","SHARED_GUESS","conjecture"} {if !strings.Contains(remote,marker){t.Fatalf("lost authorized material %s: %s",marker,remote)}}
 group.Channel=character.ChannelScene
 local:=groupConversationContext(state,group,[]string{"a","b"},scene)
 if !strings.Contains(local,"LOCAL_ROOM") || !strings.Contains(local,"SHARED_FACT") {t.Fatalf("lost local common material: %s",local)}
 if strings.Contains(local,"LOCAL_BYSTANDER") || strings.Contains(local,"SECRET_EVENT") {t.Fatalf("unprojected local metadata: %s",local)}
 if got:=groupConversationContext(state,group,[]string{"a","absent"},scene);got!="" {t.Fatalf("invented co-presence: %s",got)}
 if got:=groupConversationContext(state,group,nil,scene);got!="" {t.Fatalf("empty audience got facts: %s",got)}
 if scene.Title!="SECRET_EVENT" || len(scene.Present)!=3 {t.Fatal("projection mutated committed scene")}
}
