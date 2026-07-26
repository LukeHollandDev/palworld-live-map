using Newtonsoft.Json.Linq;

internal static partial class TestProgram
{
    private static void ExpandedBossRowsJoinAndDeduplicate()
    {
        var fixture = CreateBossAdditionFixture();
        var locations = WorldCatalogueShaper.ShapeBossAdditions(
            fixture.BossRows,
            fixture.HumanRows,
            fixture.HumanNameRows,
            fixture.RegionRows,
            fixture.IconRows);

        Equal(36, locations.Length, "expanded encounter count");
        Equal(33, locations.Count(item => item.Category == "bounties"), "Bounty count");
        Equal(3, locations.Count(item => item.Category == "oil-rigs"), "oil-rig count");

        var pinch = locations.Single(item => item.GameId == "BOSS_Police_Old");
        Equal("Pinch", pinch.Name, "case-insensitive Bounty name");
        Equal("BOSS_POLICE_OLD", pinch.StateKey, "Bounty state key");
        Equal(
            "/Game/Pal/Texture/PalIcon/NPC/T_BOSS_Police_Old.T_BOSS_Police_Old",
            pinch.IconSource,
            "Bounty icon source");

        var oilRig = locations.Single(item => item.GameId == "REGION_Oilrig_1");
        Equal("Rayne Syndicate Oil Rig", oilRig.Name, "oil-rig localized name");
        Equal(null, oilRig.StateKey, "oil-rig state key");
    }

    private static void ExpandedBossJoinsRejectCaseCollisions()
    {
        var fixture = CreateBossAdditionFixture();
        fixture.HumanRows["BOSS_POLICE_OLD"] = ((JObject)fixture.HumanRows["BOSS_Police_old"]!).DeepClone();
        Throws(
            () => WorldCatalogueShaper.ShapeBossAdditions(
                fixture.BossRows,
                fixture.HumanRows,
                fixture.HumanNameRows,
                fixture.RegionRows,
                fixture.IconRows),
            "expected one case-insensitive row");
    }

    private static void PlacedCatalogueRecordsRetainStateKeys()
    {
        const string rawGuid = "01234567-89abcdef-01234567-89abcdef";
        var names = new JObject
        {
            ["WatchTower_1"] = TextRow("Windswept Watchtower"),
            ["Waypoint_1"] = TextRow("Plateau of Beginnings")
        };
        var watchtower = WorldCatalogueShaper.ShapeFastTravel(
            Actor(
                "BP_LevelObject_UnlockMapPoint_C",
                new JObject
                {
                    ["FastTravelPointID"] = "WatchTower_1",
                    ["LevelObjectInstanceId"] = rawGuid
                }),
            names);
        Equal("watchtowers", watchtower.Category, "watchtower category");
        Equal("0123456789ABCDEF0123456789ABCDEF", watchtower.StateKey, "watchtower normalized state key");
        Equal(rawGuid.ToUpperInvariant(), watchtower.InstanceId, "watchtower raw instance ID");

        var waypoint = WorldCatalogueShaper.ShapeFastTravel(
            Actor(
                "BP_LevelObject_TowerFastTravelPoint_C",
                new JObject
                {
                    ["FastTravelPointID"] = Handle("Waypoint_1"),
                    ["LevelObjectInstanceId"] = "11111111-22222222-33333333-44444444"
                }),
            names);
        Equal("waypoints", waypoint.Category, "waypoint category");
        Equal("Plateau of Beginnings", waypoint.Name, "waypoint name");

        var dungeon = WorldCatalogueShaper.ShapeDungeon(
            Actor(
                "BP_DungeonPortalMarker_Viking_B_C",
                new JObject
                {
                    ["LevelObjectInstanceId"] = "AAAAAAAA-BBBBBBBB-CCCCCCCC-DDDDDDDD"
                }));
        Equal("dungeon-entrances", dungeon.Category, "dungeon category");
        Equal("Viking_B", dungeon.GameId, "dungeon biome");
        Equal("Biome: Viking B", dungeon.Detail, "dungeon detail");

        var effigy = WorldCatalogueShaper.ShapeEffigy(
            Actor(
                "BP_LevelObject_Relic_Penguin_C",
                new JObject
                {
                    ["LevelObjectInstanceId"] = "ABCDEF01-ABCDEF02-ABCDEF03-ABCDEF04"
                }),
            new JObject { ["PAL_NAME_Penguin"] = TextRow("Pengullet") });
        Equal("Pengullet Effigy", effigy.Name, "Effigy name");
        Equal("BP_LevelObject_Relic_Penguin", effigy.GameId, "Effigy class key");

        var lifmunk = WorldCatalogueShaper.ShapeEffigy(
            Actor(
                "BP_LevelObject_Relic_C",
                new JObject
                {
                    ["LevelObjectInstanceId"] = "10000000-20000000-30000000-40000000"
                }),
            new JObject());
        Equal("Lifmunk Effigy", lifmunk.Name, "base-class Effigy name");
    }

    private static void CollectibleAndNpcRecordsJoinGameTables()
    {
        var journal = WorldCatalogueShaper.ShapeJournal(
            Actor(
                "BP_LevelObject_Note_C",
                new JObject
                {
                    ["NoteRowName"] = Handle("GrassBoss1"),
                    ["LevelObjectInstanceId"] = "01010101-02020202-03030303-04040404"
                }),
            new JObject
            {
                ["GrassBoss1"] = TextRow(
                    "Zoe Rayne's Diary - 1\r\n\r\nA first line.\r\nA second line with |markup|.")
            },
            new JObject
            {
                ["GrassBoss1"] = new JObject
                {
                    ["Texture"] = new JObject
                    {
                        ["AssetPathName"] = "/Game/Pal/Texture/Note/T_GrassBoss1.T_GrassBoss1"
                    }
                }
            });
        Equal("Zoe Rayne's Diary - 1", journal.Name, "Journal title");
        Equal("A first line. A second line with markup.", journal.Detail, "Journal preview");
        Equal("GrassBoss1", journal.StateKey, "Journal save-state key");

        var shrine = WorldCatalogueShaper.ShapeAncientShrinePickup(
            Actor(
                "BP_LevelObject_ItemPickupTower_C",
                new JObject
                {
                    ["ItemPickupRowName"] = Handle("Tower_Grass_01"),
                    ["LevelObjectInstanceId"] = "11112222-33334444-55556666-77778888"
                }),
            new JObject
            {
                ["Tower_Grass_01"] = new JObject
                {
                    ["Item_01_Id"] = "Blueprint_OldRevolver_2",
                    ["Item_01_Num"] = 1,
                    ["Item_02_Id"] = "DogCoin",
                    ["Item_02_Num"] = 30
                }
            },
            new JObject
            {
                ["Blueprint_OldRevolver_2"] = new JObject { ["IconName"] = "Material_Blueprint" },
                ["DogCoin"] = new JObject { ["IconName"] = "DogCoin" }
            },
            new JObject
            {
                ["ITEM_NAME_Blueprint_OldRevolver_2_TextData"] = TextRow("Old Revolver Schematic 2"),
                ["ITEM_NAME_DogCoin"] = TextRow("Dog Coin")
            },
            new JObject
            {
                ["Material_Blueprint"] = IconRow("/Game/Pal/Texture/Item/T_Blueprint.T_Blueprint"),
                ["DogCoin"] = IconRow("/Game/Pal/Texture/Item/T_DogCoin.T_DogCoin")
            });
        Equal("ancient-shrine-pickups", shrine.Category, "Shrine category");
        Equal("Old Revolver Schematic 2", shrine.Name, "Shrine primary reward");
        Equal("+30 Dog Coin", shrine.Detail, "Shrine secondary reward");
        Equal(2, shrine.Rewards?.Length, "structured Shrine rewards");
        Equal("localized", shrine.Rewards?[0].NameSource, "Shrine reward name provenance");

        var medalTrader = WorldCatalogueShaper.ShapeNpc(
            Actor("BP_MonoNPCSpawner_MedalTrader_C", new JObject()),
            new JObject
            {
                ["MedalTrader"] = new JObject { ["NameTextID"] = "NPC_NAME_MedalTrader" }
            },
            new JObject
            {
                ["NPC_NAME_MedalTrader"] = TextRow("Medal Merchant")
            },
            out var medalExclusion) ?? throw new InvalidOperationException("Expected Medal Trader output.");
        Equal(null, medalExclusion, "Medal Trader exclusion");
        Equal("MedalTrader", medalTrader.GameId, "Medal Trader class default identity");
        Equal(50, medalTrader.Level, "Medal Trader class default level");
        Equal("Dog Coin", medalTrader.Detail, "Medal Trader category");

        var excluded = WorldCatalogueShaper.ShapeNpc(
            Actor(
                "BP_MonoNPCSpawner_C",
            new JObject { ["UniqueName"] = Handle("U_Reward_Test") }),
            new JObject(),
            new JObject(),
            out var exclusionReason);
        Equal(null, excluded, "reward-spawner exclusion");
        Equal("non-character reward or emote spawner", exclusionReason, "reward-spawner exclusion reason");
    }

    private static void PlacedCatalogueRecordsRejectMalformedIds()
    {
        var names = new JObject { ["Waypoint_1"] = TextRow("Waypoint") };
        Throws(
            () => WorldCatalogueShaper.ShapeFastTravel(
                Actor(
                    "BP_LevelObject_TowerFastTravelPoint_C",
                    new JObject
                    {
                        ["FastTravelPointID"] = "Waypoint_1",
                        ["LevelObjectInstanceId"] = "not-a-guid"
                    }),
                names),
            "32-hex-character instance GUID");
    }

    private static BossAdditionFixture CreateBossAdditionFixture()
    {
        var bossRows = new JObject();
        var humanRows = new JObject();
        var humanNameRows = new JObject();
        var regionRows = new JObject();
        var iconRows = new JObject();

        for (var index = 0; index < 90; index++)
        {
            bossRows[$"pal-{index:D3}"] = BossRow(
                $"Alpha_{index:D3}",
                $"Pal_{index:D3}",
                1_000 + index,
                2_000 + index,
                100,
                10);
        }
        for (var index = 0; index < 33; index++)
        {
            var spawnerId = index == 0 ? "BOSS_Police_Old" : $"BOSS_Test_{index:D2}";
            var nameTextId = index == 0 ? "NPC_NAME_PINCH" : $"NPC_NAME_{index:D2}";
            var humanKey = index == 0 ? "BOSS_Police_old" : spawnerId;
            var nameKey = index == 0 ? "npc_name_pinch" : nameTextId;
            var iconKey = index == 0 ? "boss_police_old" : spawnerId;
            var row = BossRow(spawnerId, "None", -10_000 - index, 20_000 + index, 50, 20 + index);
            bossRows[$"human-a-{index:D2}"] = row;
            bossRows[$"human-b-{index:D2}"] = row.DeepClone();
            humanRows[humanKey] = new JObject { ["OverrideNameTextID"] = nameTextId };
            humanNameRows[nameKey] = TextRow(index == 0 ? "Pinch" : $"Bounty {index:D2}");
            iconRows[iconKey] = IconRow(
                $"/Game/Pal/Texture/PalIcon/NPC/T_{spawnerId}.T_{spawnerId}");
        }
        for (var index = 1; index <= 3; index++)
        {
            var spawnerId = $"REGION_Oilrig_{index}";
            bossRows[$"oil-{index}"] = BossRow(
                spawnerId,
                "None",
                30_000 + index,
                -40_000 - index,
                0,
                30 + index);
            regionRows[spawnerId] = TextRow(index == 1
                ? "Rayne Syndicate Oil Rig"
                : $"Oil Rig {index}");
        }
        return new BossAdditionFixture(bossRows, humanRows, humanNameRows, regionRows, iconRows);
    }

    private static JObject BossRow(
        string spawnerId,
        string characterId,
        double x,
        double y,
        double z,
        int level) =>
        new()
        {
            ["SpawnerID"] = spawnerId,
            ["CharacterID"] = characterId,
            ["Level"] = level,
            ["Location"] = new JObject
            {
                ["X"] = x,
                ["Y"] = y,
                ["Z"] = z
            }
        };

    private static PlacedActor Actor(string className, JObject properties) =>
        new(
            $"{className}_fixture",
            className,
            "/Game/Test/Fixture.Fixture",
            properties,
            123.25,
            -456.5,
            78.75);

    private static JObject Handle(string key) => new() { ["Key"] = key };

    private static JObject TextRow(string value) =>
        new()
        {
            ["TextData"] = new JObject
            {
                ["LocalizedString"] = value
            }
        };

    private static JObject IconRow(string assetPath) =>
        new()
        {
            ["Icon"] = new JObject
            {
                ["AssetPathName"] = assetPath
            }
        };

    private sealed record BossAdditionFixture(
        JObject BossRows,
        JObject HumanRows,
        JObject HumanNameRows,
        JObject RegionRows,
        JObject IconRows);
}
