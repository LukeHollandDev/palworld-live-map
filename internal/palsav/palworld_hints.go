// Copyright (C) 2026 Luke Holland
//
// Adapted on 2026-07-23 from PalworldSaveTools. See NOTICE for the exact
// source revision and provenance.
// SPDX-License-Identifier: GPL-3.0-or-later

package palsav

// Palworld's legacy property tags omit the concrete identity of StructProperty
// map keys/values and set elements. These hints cover the current save schema
// exercised by the supplied July 2026 saves and the maintained Palworld tools.
var palworldTypeHints = map[string]string{
	".worldSaveData.CharacterContainerSaveData.Key":                                                                    "StructProperty",
	".worldSaveData.CharacterContainerSaveData.Value":                                                                  "StructProperty",
	".worldSaveData.CharacterSaveParameterMap.Key":                                                                     "StructProperty",
	".worldSaveData.CharacterSaveParameterMap.Value":                                                                   "StructProperty",
	".worldSaveData.FoliageGridSaveDataMap.Key":                                                                        "StructProperty",
	".worldSaveData.FoliageGridSaveDataMap.Value":                                                                      "StructProperty",
	".worldSaveData.FoliageGridSaveDataMap.Value.ModelMap.Value":                                                       "StructProperty",
	".worldSaveData.FoliageGridSaveDataMap.Value.ModelMap.Value.InstanceDataMap.Key":                                   "StructProperty",
	".worldSaveData.FoliageGridSaveDataMap.Value.ModelMap.Value.InstanceDataMap.Value":                                 "StructProperty",
	".worldSaveData.ItemContainerSaveData.Key":                                                                         "StructProperty",
	".worldSaveData.ItemContainerSaveData.Value":                                                                       "StructProperty",
	".worldSaveData.MapObjectSaveData.MapObjectSaveData.ConcreteModel.ModuleMap.Value":                                 "StructProperty",
	".worldSaveData.MapObjectSaveData.MapObjectSaveData.Model.EffectMap.Value":                                         "StructProperty",
	".worldSaveData.MapObjectSpawnerInStageSaveData.Key":                                                               "StructProperty",
	".worldSaveData.MapObjectSpawnerInStageSaveData.Value":                                                             "StructProperty",
	".worldSaveData.MapObjectSpawnerInStageSaveData.Value.SpawnerDataMapByLevelObjectInstanceId.Key":                   "Guid",
	".worldSaveData.MapObjectSpawnerInStageSaveData.Value.SpawnerDataMapByLevelObjectInstanceId.Value":                 "StructProperty",
	".worldSaveData.MapObjectSpawnerInStageSaveData.Value.SpawnerDataMapByLevelObjectInstanceId.Value.ItemMap.Value":   "StructProperty",
	".worldSaveData.WorkSaveData.WorkSaveData.WorkAssignMap.Value":                                                     "StructProperty",
	".worldSaveData.BaseCampSaveData.Key":                                                                              "Guid",
	".worldSaveData.BaseCampSaveData.Value":                                                                            "StructProperty",
	".worldSaveData.BaseCampSaveData.Value.ModuleMap.Value":                                                            "StructProperty",
	".worldSaveData.GroupSaveDataMap.Key":                                                                              "Guid",
	".worldSaveData.GroupSaveDataMap.Value":                                                                            "StructProperty",
	".worldSaveData.GuildExtraSaveDataMap.Key":                                                                         "Guid",
	".worldSaveData.GuildExtraSaveDataMap.Value":                                                                       "StructProperty",
	".worldSaveData.LockGimmickSaveData.Key":                                                                           "Guid",
	".worldSaveData.LockGimmickSaveData.Value":                                                                         "StructProperty",
	".worldSaveData.FishingSpotSaveData.Key":                                                                           "Guid",
	".worldSaveData.FishingSpotSaveData.Value":                                                                         "StructProperty",
	".worldSaveData.EnemyCampSaveData.EnemyCampStatusMap.Value":                                                        "StructProperty",
	".worldSaveData.EnemyCampSaveData.EnemyCampStatusMap.Value.TreasureBoxInfoMapBySpawnerName.Value":                  "StructProperty",
	".worldSaveData.DungeonSaveData.DungeonSaveData.MapObjectSaveData.MapObjectSaveData.Model.EffectMap.Value":         "StructProperty",
	".worldSaveData.DungeonSaveData.DungeonSaveData.MapObjectSaveData.MapObjectSaveData.ConcreteModel.ModuleMap.Value": "StructProperty",
	".worldSaveData.DungeonSaveData.DungeonSaveData.RewardSaveDataMap.Key":                                             "Guid",
	".worldSaveData.DungeonSaveData.DungeonSaveData.RewardSaveDataMap.Value":                                           "StructProperty",
	".worldSaveData.InvaderSaveData.Key":                                                                               "Guid",
	".worldSaveData.InvaderSaveData.Value":                                                                             "StructProperty",
	".worldSaveData.InvaderDeclarationSaveData.ValidatedStartPointIds.StructProperty":                                  "Guid",
	".worldSaveData.OilrigSaveData.OilrigMap.Value":                                                                    "StructProperty",
	".worldSaveData.SupplySaveData.SupplyInfos.Key":                                                                    "Guid",
	".worldSaveData.SupplySaveData.SupplyInfos.Value":                                                                  "StructProperty",
	".SaveData.Local_MaxFriendshipPalIds.Key":                                                                          "StructProperty",
	".SaveData.Local_MaxFriendshipPalIds.Value":                                                                        "StructProperty",
	".SaveData.RecordData.FoundTreasureMapPointMap.Key":                                                                "Guid",
}
