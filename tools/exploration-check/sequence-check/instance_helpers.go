package main

import "ta/internal/exploration"

func actionPermit(def exploration.DungeonDefinition, scope exploration.DungeonScope, snapshot exploration.DungeonInstanceSnapshot, action exploration.InstanceActionKind, target, from, to, source string) exploration.InstanceActionPermit {
	return exploration.InstanceActionPermit{
		Binding:  exploration.InstancePermitBinding{InstanceID: snapshot.Binding.InstanceID, WorldID: scope.WorldID, BranchID: scope.BranchID, DefinitionHash: snapshot.DefinitionHash, Revision: snapshot.Revision},
		Decision: exploration.DecisionAllowed, Action: action, TargetID: target, FromAreaID: from, ToAreaID: to, SourceRef: source,
	}
}

func actionContext(scope exploration.DungeonScope, snapshot exploration.DungeonInstanceSnapshot, permit exploration.InstanceActionPermit) exploration.InstanceActionContext {
	return exploration.InstanceActionContext{ExpectedRevision: snapshot.Revision, Scope: scope, Permit: permit, Battle: exploration.BattleNone, Blocking: exploration.BlockingNone}
}

func createInstance(def exploration.DungeonDefinition, scope exploration.DungeonScope, instanceID, entryID, source string) (exploration.InstanceTransition, error) {
	hash, err := exploration.DungeonDefinitionContentHash(def)
	if err != nil {
		return exploration.InstanceTransition{}, err
	}
	permit := exploration.InstanceActionPermit{
		Binding:  exploration.InstancePermitBinding{InstanceID: instanceID, WorldID: scope.WorldID, BranchID: scope.BranchID, DefinitionHash: hash, Revision: 0},
		Decision: exploration.DecisionAllowed, Action: exploration.ActionEnter, TargetID: entryID, SourceRef: source,
	}
	return exploration.CreateDungeonInstance(exploration.CreateDungeonInstanceInput{Definition: def, Scope: scope, InstanceID: instanceID, EntryAreaID: entryID,
		Permit: permit, Battle: exploration.BattleNone, Blocking: exploration.BlockingNone,
		InitialDiscoveries: []exploration.InstanceDiscovery{{Kind: exploration.DiscoveryObjective, ID: "recover.inscription", SourceRef: source}}})
}
