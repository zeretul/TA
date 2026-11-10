package cultivation

// These are physical adaptation sources. A manual grants neither cultivation
// nor skills; its holder still pays the published study time and meets the floor.
var spiritWorldItems = []ItemDef{
	{ID: "material.spirit.boundary_anchor", NameKey: "item.spirit.boundary_anchor.name", Kind: KindMaterial, Grade: GradeTop, Rarity: RarityRare, DefinitionDetails: DefinitionDetails{QualityGrade: QualityPurple}, Material: &MaterialDef{UnitKey: "item.unit.piece"}},
	{ID: "material.spirit.boundary_jade", NameKey: "item.spirit.boundary_jade.name", Kind: KindMaterial, Grade: GradeTop, Rarity: RarityRare, BasePrice: 8000, DefinitionDetails: DefinitionDetails{QualityGrade: QualityPurple}, Material: &MaterialDef{UnitKey: "item.unit.block"}},
	{ID: "manual.spirit.void", NameKey: "item.spirit.void_manual.name", Kind: KindMaterial, Grade: GradeTop, Rarity: RarityRare, BasePrice: 12500, DefinitionDetails: DefinitionDetails{QualityGrade: QualityOrange}, Material: &MaterialDef{UnitKey: "item.unit.piece"}},
	{ID: "manual.spirit.integration", NameKey: "item.spirit.integration_manual.name", Kind: KindMaterial, Grade: GradeTop, Rarity: RarityRare, BasePrice: 18000, DefinitionDetails: DefinitionDetails{QualityGrade: QualityOrange}, Material: &MaterialDef{UnitKey: "item.unit.piece"}},
	{ID: "manual.spirit.mahayana", NameKey: "item.spirit.mahayana_manual.name", Kind: KindMaterial, Grade: GradeTop, Rarity: RarityPrecious, BasePrice: 24500, DefinitionDetails: DefinitionDetails{QualityGrade: QualityRed}, Material: &MaterialDef{UnitKey: "item.unit.piece"}},
	{ID: "material.spirit.rift_sample", NameKey: "item.spirit.rift_sample.name", Kind: KindMaterial, Grade: GradeTop, Rarity: RarityRare, DefinitionDetails: DefinitionDetails{QualityGrade: QualityOrange}, Material: &MaterialDef{UnitKey: "item.unit.block"}},
	{ID: "material.spirit.formation_record", NameKey: "item.spirit.formation_record.name", Kind: KindMaterial, Grade: GradeTop, Rarity: RarityRare, DefinitionDetails: DefinitionDetails{QualityGrade: QualityOrange}, Material: &MaterialDef{UnitKey: "item.unit.piece"}},
	{ID: "material.spirit.ancient_rubbing", NameKey: "item.spirit.ancient_rubbing.name", Kind: KindMaterial, Grade: GradeTop, Rarity: RarityPrecious, DefinitionDetails: DefinitionDetails{QualityGrade: QualityRed}, Material: &MaterialDef{UnitKey: "item.unit.piece"}},
}
