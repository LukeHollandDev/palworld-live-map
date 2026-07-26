using System.Globalization;
using System.Text.RegularExpressions;
using Newtonsoft.Json.Linq;

// Pure joins and schema shaping for the expanded world catalogue. CUE4Parse
// objects are reduced to PlacedActor before entering this layer so every join
// and fail-closed rule can be covered by small fixtures.
internal static partial class WorldCatalogueShaper
{
    private const string BossSpawnerPackage = "Pal/Content/Pal/DataTable/UI/DT_BossSpawnerLoactionData";
    private const string HumanParameterPackage = "Pal/Content/Pal/DataTable/Character/DT_PalHumanParameter";
    private const string HumanNamePackage = "Pal/Content/L10N/en/Pal/DataTable/Text/DT_HumanNameText_Common";
    private const string RegionNamePackage = "Pal/Content/L10N/en/Pal/DataTable/Text/DT_WorldMap_Common_Text_Common";
    private const string BossIconPackage = "Pal/Content/Pal/DataTable/Character/DT_PalBossNPCIcon";
    private const string RespawnNamePackage = "Pal/Content/L10N/en/Pal/DataTable/Text/DT_MapRespawnPointInfoText";
    private const string PalNamePackage = "Pal/Content/L10N/en/Pal/DataTable/Text/DT_PalNameText_Common";
    private const string NoteDescPackage = "Pal/Content/L10N/en/Pal/DataTable/Text/DT_NoteDescText";
    private const string NoteTexturePackage = "Pal/Content/Pal/DataTable/NoteData/DT_NoteTextureDataTable";
    private const string ItemPickupPackage = "Pal/Content/Pal/DataTable/Item/DT_ItemPickupDataTable";
    private const string ItemDataPackage = "Pal/Content/Pal/DataTable/Item/DT_ItemDataTable";
    private const string ItemIconPackage = "Pal/Content/Pal/DataTable/Item/DT_ItemIconDataTable";
    private const string ItemNamePackage = "Pal/Content/L10N/en/Pal/DataTable/Text/DT_ItemNameText_Common";
    private const string UniqueNpcPackage = "Pal/Content/Pal/DataTable/Character/DT_UniqueNPC";
    private const string UniqueNpcNamePackage = "Pal/Content/L10N/en/Pal/DataTable/Text/DT_UniqueNPCText_Common";

    private const string OilRigIcon =
        "/Game/Pal/Texture/UI/InGame/T_icon_compass_Oilrig.T_icon_compass_Oilrig";
    private const string WatchtowerIcon =
        "/Game/Pal/Texture/UI/InGame/T_icon_compass_FTUnlockMap.T_icon_compass_FTUnlockMap";
    private const string WaypointIcon =
        "/Game/Pal/Texture/UI/InGame/T_icon_compass_FTtower.T_icon_compass_FTtower";
    private const string DungeonIcon =
        "/Game/Pal/Texture/UI/InGame/T_icon_compass_dungeon.T_icon_compass_dungeon";

    private static readonly HashSet<string> ExpectedOilRigIds =
        new(StringComparer.Ordinal)
        {
            "REGION_Oilrig_1",
            "REGION_Oilrig_2",
            "REGION_Oilrig_3"
        };

    private static readonly IReadOnlyDictionary<string, int> ExpectedPersistentDungeonBiomes =
        new Dictionary<string, int>(StringComparer.Ordinal)
        {
            ["Desert"] = 15,
            ["Forest"] = 30,
            ["Grass1"] = 35,
            ["Sakura"] = 15,
            ["Skyland"] = 1,
            ["Snow"] = 15,
            ["Viking"] = 6,
            ["Viking_B"] = 8,
            ["Viking_C"] = 16,
            ["Volcano"] = 15,
            ["Yakushima"] = 1
        };

    private static readonly IReadOnlyDictionary<string, int> ExpectedCompleteDungeonBiomes =
        new Dictionary<string, int>(StringComparer.Ordinal)
        {
            ["Desert"] = 15,
            ["Forest"] = 30,
            ["Grass1"] = 48,
            ["Sakura"] = 15,
            ["Skyland"] = 1,
            ["Snow"] = 15,
            ["Viking"] = 6,
            ["Viking_B"] = 8,
            ["Viking_C"] = 16,
            ["Volcano"] = 15,
            ["Yakushima"] = 1
        };

    private static readonly IReadOnlyDictionary<string, int> ExpectedEffigyClasses =
        new Dictionary<string, int>(StringComparer.Ordinal)
        {
            ["BP_LevelObject_Relic_C"] = 155,
            ["BP_LevelObject_Relic_FlameBambi_C"] = 30,
            ["BP_LevelObject_Relic_GuardianDog_C"] = 4,
            ["BP_LevelObject_Relic_IceCrocodile_C"] = 30,
            ["BP_LevelObject_Relic_LazyDragon_C"] = 4,
            ["BP_LevelObject_Relic_LeafMomonga_C"] = 30,
            ["BP_LevelObject_Relic_Monkey_C"] = 30,
            ["BP_LevelObject_Relic_Mutant_C"] = 4,
            ["BP_LevelObject_Relic_NegativeKoala_C"] = 30,
            ["BP_LevelObject_Relic_Penguin_C"] = 30,
            ["BP_LevelObject_Relic_PinkCat_C"] = 30,
            ["BP_LevelObject_Relic_SheepBall_C"] = 30
        };

    internal static WorldCatalogueLocation[] ShapeBossAdditions(
        JObject bossRows,
        JObject humanRows,
        JObject humanNameRows,
        JObject regionNameRows,
        JObject bossIconRows)
    {
        var locations = new Dictionary<string, WorldCatalogueLocation>(StringComparer.Ordinal);
        var duplicateBounties = 0;
        var palRows = 0;
        var rawBountyRows = 0;
        var rawOilRigRows = 0;

        foreach (var property in bossRows.Properties().OrderBy(item => item.Name, StringComparer.Ordinal))
        {
            var context = $"{BossSpawnerPackage}[{property.Name}]";
            var row = LandmarkShaper.RequireObject(property.Value, context);
            var spawnerId = LandmarkShaper.RequireString(row["SpawnerID"], $"{context}.SpawnerID");
            var characterId = LandmarkShaper.NormalizeEnum(
                LandmarkShaper.RequireString(row["CharacterID"], $"{context}.CharacterID"));

            WorldCatalogueLocation? location = null;
            if (spawnerId.StartsWith("REGION_Oilrig", StringComparison.OrdinalIgnoreCase))
            {
                rawOilRigRows++;
                if (characterId != "None")
                {
                    throw new InvalidOperationException($"{context} is an oil rig but has CharacterID {characterId}.");
                }
                var region = LandmarkShaper.RequireRow(regionNameRows, spawnerId, RegionNamePackage);
                location = ShapeBossTableLocation(
                    property.Name,
                    row,
                    "oil-rigs",
                    spawnerId,
                    LandmarkShaper.ResolveLocalizedText(region, $"{RegionNamePackage}[{spawnerId}]"),
                    "Oil rig",
                    null,
                    OilRigIcon);
            }
            else if (characterId == "None")
            {
                rawBountyRows++;
                var human = RequireRowIgnoreCase(humanRows, spawnerId, HumanParameterPackage);
                var nameTextId = RequireName(human["OverrideNameTextID"], $"{HumanParameterPackage}[{spawnerId}].OverrideNameTextID");
                if (LandmarkShaper.NormalizeEnum(nameTextId) == "None")
                {
                    throw new InvalidOperationException($"{HumanParameterPackage}[{spawnerId}] has no OverrideNameTextID.");
                }
                var nameRow = RequireRowIgnoreCase(humanNameRows, nameTextId, HumanNamePackage);
                var iconRow = RequireRowIgnoreCase(bossIconRows, spawnerId, BossIconPackage);
                var icon = LandmarkShaper.RequireObject(iconRow["Icon"], $"{BossIconPackage}[{spawnerId}].Icon");
                var iconSource = LandmarkShaper.RequireString(
                    icon["AssetPathName"],
                    $"{BossIconPackage}[{spawnerId}].Icon.AssetPathName");
                location = ShapeBossTableLocation(
                    property.Name,
                    row,
                    "bounties",
                    spawnerId,
                    LandmarkShaper.ResolveLocalizedText(nameRow, $"{HumanNamePackage}[{nameTextId}]"),
                    "Bounty",
                    spawnerId.ToUpperInvariant(),
                    iconSource);
            }
            else
            {
                palRows++;
            }

            if (location == null)
            {
                continue;
            }
            if (locations.TryGetValue(location.Id, out var previous))
            {
                if (location.Category == "bounties" && EquivalentExceptSourceObject(previous, location))
                {
                    duplicateBounties++;
                    continue;
                }
                throw new InvalidOperationException(
                    $"Conflicting boss-table records generated duplicate catalogue ID {location.Id}.");
            }
            locations.Add(location.Id, location);
        }

        var result = locations.Values
            .OrderBy(item => item.Category, StringComparer.Ordinal)
            .ThenBy(item => item.Id, StringComparer.Ordinal)
            .ToArray();
        RequireCount("Pal Alpha boss-table rows", 90, palRows);
        RequireCount("raw Bounty boss-table rows", 66, rawBountyRows);
        RequireCount("deduplicated Bounty rows", 33, result.Count(item => item.Category == "bounties"));
        RequireCount("exact duplicate Bounty rows", 33, duplicateBounties);
        RequireCount("oil-rig boss-table rows", 3, rawOilRigRows);
        var oilRigIds = result
            .Where(item => item.Category == "oil-rigs")
            .Select(item => item.GameId)
            .ToHashSet(StringComparer.Ordinal);
        if (!oilRigIds.SetEquals(ExpectedOilRigIds))
        {
            throw new InvalidOperationException(
                $"Unexpected oil-rig SpawnerIDs: {string.Join(", ", oilRigIds.OrderBy(item => item, StringComparer.Ordinal))}.");
        }
        AssertValidLocations(result);
        return result;
    }

    internal static WorldCatalogueLocation ShapeFastTravel(
        PlacedActor actor,
        JObject respawnNameRows)
    {
        var (category, prefix, detail, icon) = actor.ClassName switch
        {
            "BP_LevelObject_UnlockMapPoint_C" =>
                ("watchtowers", "watchtower", "Watchtower", WatchtowerIcon),
            "BP_LevelObject_TowerFastTravelPoint_C" =>
                ("waypoints", "waypoint", "Fast travel point", WaypointIcon),
            _ => throw new InvalidOperationException(
                $"Actor {actor.ActorName} has unsupported fast-travel class {actor.ClassName}.")
        };
        var context = $"{actor.ClassName} actor {actor.ActorName}";
        var pointId = RequireName(actor.Properties["FastTravelPointID"], $"{context}.FastTravelPointID");
        var instanceId = RequireInstanceId(actor.Properties["LevelObjectInstanceId"], $"{context}.LevelObjectInstanceId");
        var row = LandmarkShaper.RequireRow(respawnNameRows, pointId, RespawnNamePackage);
        return WithActorSource(
            new WorldCatalogueLocation(
                $"{prefix}:{pointId}",
                category,
                pointId,
                LandmarkShaper.ResolveLocalizedText(row, $"{RespawnNamePackage}[{pointId}]"),
                actor.X,
                actor.Y,
                actor.Z)
            {
                Detail = detail,
                InstanceId = instanceId.Raw,
                StateKey = instanceId.Normalized,
                ClassName = actor.ClassName,
                IconSource = icon
            },
            actor);
    }

    internal static WorldCatalogueLocation ShapeDungeon(PlacedActor actor)
    {
        const string prefix = "BP_DungeonPortalMarker_";
        if (!actor.ClassName.StartsWith(prefix, StringComparison.Ordinal) ||
            !actor.ClassName.EndsWith("_C", StringComparison.Ordinal))
        {
            throw new InvalidOperationException(
                $"Actor {actor.ActorName} has unsupported dungeon class {actor.ClassName}.");
        }
        var biome = actor.ClassName[prefix.Length..^2];
        if (string.IsNullOrWhiteSpace(biome))
        {
            throw new InvalidOperationException($"Dungeon actor {actor.ActorName} has an empty biome.");
        }
        var instanceId = RequireInstanceId(
            actor.Properties["LevelObjectInstanceId"],
            $"{actor.ClassName} actor {actor.ActorName}.LevelObjectInstanceId");
        return WithActorSource(
            new WorldCatalogueLocation(
                $"dungeon:{instanceId.Normalized}",
                "dungeon-entrances",
                biome,
                "Dungeon entrance",
                actor.X,
                actor.Y,
                actor.Z)
            {
                Detail = $"Biome: {ReadableIdentifier(biome)}",
                InstanceId = instanceId.Raw,
                StateKey = instanceId.Normalized,
                ClassName = actor.ClassName,
                IconSource = DungeonIcon
            },
            actor);
    }

    internal static WorldCatalogueLocation ShapeEffigy(PlacedActor actor, JObject palNameRows)
    {
        const string baseClass = "BP_LevelObject_Relic";
        var classKey = actor.ClassName.EndsWith("_C", StringComparison.Ordinal)
            ? actor.ClassName[..^2]
            : actor.ClassName;
        if (!classKey.StartsWith(baseClass, StringComparison.Ordinal))
        {
            throw new InvalidOperationException(
                $"Actor {actor.ActorName} has unsupported Effigy class {actor.ClassName}.");
        }

        string palName;
        if (classKey == baseClass)
        {
            // The base Blueprint has no Pal suffix; this is the one reviewed
            // class-name exception. Its placed object is Lifmunk's Effigy.
            palName = "Lifmunk";
        }
        else
        {
            if (!classKey.StartsWith(baseClass + "_", StringComparison.Ordinal))
            {
                throw new InvalidOperationException(
                    $"Effigy actor {actor.ActorName} has an unrecognized class name {actor.ClassName}.");
            }
            var palKey = classKey[(baseClass + "_").Length..];
            if (string.IsNullOrWhiteSpace(palKey))
            {
                throw new InvalidOperationException($"Effigy actor {actor.ActorName} has an empty Pal key.");
            }
            var nameRowKey = $"PAL_NAME_{palKey}";
            var nameRow = LandmarkShaper.RequireRow(palNameRows, nameRowKey, PalNamePackage);
            palName = LandmarkShaper.ResolveLocalizedText(nameRow, $"{PalNamePackage}[{nameRowKey}]");
        }

        var instanceId = RequireInstanceId(
            actor.Properties["LevelObjectInstanceId"],
            $"{actor.ClassName} actor {actor.ActorName}.LevelObjectInstanceId");
        return WithActorSource(
            new WorldCatalogueLocation(
                $"effigy:{instanceId.Normalized}",
                "effigies",
                classKey,
                $"{palName} Effigy",
                actor.X,
                actor.Y,
                actor.Z)
            {
                Detail = "Effigy",
                InstanceId = instanceId.Raw,
                StateKey = instanceId.Normalized,
                ClassName = actor.ClassName
            },
            actor);
    }

    internal static WorldCatalogueLocation ShapeJournal(
        PlacedActor actor,
        JObject noteDescRows,
        JObject noteTextureRows)
    {
        if (actor.ClassName != "BP_LevelObject_Note_C")
        {
            throw new InvalidOperationException(
                $"Actor {actor.ActorName} has unsupported Journal class {actor.ClassName}.");
        }
        var context = $"{actor.ClassName} actor {actor.ActorName}";
        var noteId = RequireName(actor.Properties["NoteRowName"], $"{context}.NoteRowName");
        var instanceId = RequireInstanceId(actor.Properties["LevelObjectInstanceId"], $"{context}.LevelObjectInstanceId");
        var descRow = LandmarkShaper.RequireRow(noteDescRows, noteId, NoteDescPackage);
        var fullText = LandmarkShaper.ResolveLocalizedText(descRow, $"{NoteDescPackage}[{noteId}]");
        var (title, preview) = JournalTitleAndPreview(fullText);
        var textureRow = LandmarkShaper.RequireRow(noteTextureRows, noteId, NoteTexturePackage);
        var texture = LandmarkShaper.RequireObject(textureRow["Texture"], $"{NoteTexturePackage}[{noteId}].Texture");
        var iconSource = LandmarkShaper.RequireString(
            texture["AssetPathName"],
            $"{NoteTexturePackage}[{noteId}].Texture.AssetPathName");

        return WithActorSource(
            new WorldCatalogueLocation(
                $"journal:{noteId}:{instanceId.Normalized}",
                "journals",
                noteId,
                title,
                actor.X,
                actor.Y,
                actor.Z)
            {
                Detail = preview,
                InstanceId = instanceId.Raw,
                // Save-game note completion is keyed by NoteRowName, not GUID.
                StateKey = noteId,
                ClassName = actor.ClassName,
                IconSource = iconSource
            },
            actor);
    }

    internal static WorldCatalogueLocation ShapeAncientShrinePickup(
        PlacedActor actor,
        JObject pickupRows,
        JObject itemRows,
        JObject itemNameRows,
        JObject itemIconRows)
    {
        if (actor.ClassName != "BP_LevelObject_ItemPickupTower_C")
        {
            throw new InvalidOperationException(
                $"Actor {actor.ActorName} has unsupported ancient-shrine class {actor.ClassName}.");
        }
        var context = $"{actor.ClassName} actor {actor.ActorName}";
        var pickupId = RequireName(actor.Properties["ItemPickupRowName"], $"{context}.ItemPickupRowName");
        var instanceId = RequireInstanceId(actor.Properties["LevelObjectInstanceId"], $"{context}.LevelObjectInstanceId");
        var pickup = LandmarkShaper.RequireRow(pickupRows, pickupId, ItemPickupPackage);

        var primaryId = RequireName(pickup["Item_01_Id"], $"{ItemPickupPackage}[{pickupId}].Item_01_Id");
        if (LandmarkShaper.NormalizeEnum(primaryId) == "None")
        {
            throw new InvalidOperationException($"{ItemPickupPackage}[{pickupId}] has no primary reward.");
        }
        var primaryCount = OptionalInt(pickup["Item_01_Num"], $"{ItemPickupPackage}[{pickupId}].Item_01_Num") ?? 1;
        if (primaryCount <= 0)
        {
            throw new InvalidOperationException($"{ItemPickupPackage}[{pickupId}] has a non-positive primary reward count.");
        }

        var rewards = new List<CatalogueReward>
        {
            ShapeReward(primaryId, primaryCount, itemRows, itemNameRows, itemIconRows)
        };
        var bonusId = OptionalName(pickup["Item_02_Id"], $"{ItemPickupPackage}[{pickupId}].Item_02_Id");
        var bonusCount = OptionalInt(pickup["Item_02_Num"], $"{ItemPickupPackage}[{pickupId}].Item_02_Num") ?? 0;
        if (bonusId != null && bonusCount > 0)
        {
            rewards.Add(ShapeReward(bonusId, bonusCount, itemRows, itemNameRows, itemIconRows));
        }
        else if (bonusId != null || bonusCount != 0)
        {
            throw new InvalidOperationException(
                $"{ItemPickupPackage}[{pickupId}] has an incomplete secondary reward.");
        }

        var detail = rewards.Count == 1
            ? "Ancient Shrine pickup"
            : $"+{rewards[1].Count} {rewards[1].Name}";
        return WithActorSource(
            new WorldCatalogueLocation(
                $"ancient-shrine:{pickupId}:{instanceId.Normalized}",
                "ancient-shrine-pickups",
                pickupId,
                rewards[0].Name,
                actor.X,
                actor.Y,
                actor.Z)
            {
                Detail = detail,
                InstanceId = instanceId.Raw,
                StateKey = instanceId.Normalized,
                ClassName = actor.ClassName,
                IconSource = rewards[0].IconSource,
                Rewards = rewards.ToArray()
            },
            actor);
    }

    internal static WorldCatalogueLocation? ShapeNpc(
        PlacedActor actor,
        JObject uniqueNpcRows,
        JObject uniqueNpcNameRows,
        out string? exclusionReason)
    {
        exclusionReason = null;
        var uniqueName = OptionalName(actor.Properties["UniqueName"], $"{actor.ClassName} actor {actor.ActorName}.UniqueName");
        var defaultLevel = (int?)null;
        if (actor.ClassName == "BP_MonoNPCSpawner_MedalTrader_C")
        {
            uniqueName ??= "MedalTrader";
            defaultLevel = 50;
        }
        if (string.IsNullOrWhiteSpace(uniqueName))
        {
            exclusionReason = "unconfigured spawner (no UniqueName)";
            return null;
        }
        if (uniqueName.StartsWith("U_Reward_", StringComparison.Ordinal) ||
            uniqueName.StartsWith("U_Emote_location_", StringComparison.Ordinal))
        {
            exclusionReason = "non-character reward or emote spawner";
            return null;
        }

        var level = OptionalInt(actor.Properties["Level"], $"{actor.ClassName} actor {actor.ActorName}.Level")
            ?? defaultLevel;
        var displayName = ReadableIdentifier(uniqueName.StartsWith("U_", StringComparison.Ordinal)
            ? uniqueName["U_".Length..]
            : uniqueName);
        var npcRow = FindRow(uniqueNpcRows, uniqueName, StringComparison.Ordinal);
        if (npcRow != null)
        {
            var nameTextId = OptionalName(npcRow["NameTextID"], $"{UniqueNpcPackage}[{uniqueName}].NameTextID");
            if (nameTextId != null)
            {
                var nameRow = LandmarkShaper.RequireRow(uniqueNpcNameRows, nameTextId, UniqueNpcNamePackage);
                displayName = LandmarkShaper.ResolveLocalizedText(
                    nameRow,
                    $"{UniqueNpcNamePackage}[{nameTextId}]");
            }
        }

        var category = uniqueName.StartsWith("DarkTrader", StringComparison.Ordinal)
            ? "Black Market"
            : uniqueName == "MedalTrader"
                ? "Dog Coin"
                : "General";
        var coordinateId = string.Create(CultureInfo.InvariantCulture, $"{actor.X:F2}:{actor.Y:F2}");
        return WithActorSource(
            new WorldCatalogueLocation(
                $"npc-location:{uniqueName}:{coordinateId}",
                "npc-locations",
                uniqueName,
                displayName,
                actor.X,
                actor.Y,
                actor.Z)
            {
                Detail = category,
                Level = level,
                ClassName = actor.ClassName
            },
            actor);
    }

    internal static void AssertPersistentCounts(
        IReadOnlyCollection<WorldCatalogueLocation> fastTravel,
        IReadOnlyCollection<WorldCatalogueLocation> dungeons)
    {
        RequireCount("watchtowers", 22, fastTravel.Count(item => item.Category == "watchtowers"));
        RequireCount("waypoints", 152, fastTravel.Count(item => item.Category == "waypoints"));
        RequireCount("dungeon entrances", 157, dungeons.Count);
        AssertHistogram(
            "persistent dungeon biomes",
            ExpectedPersistentDungeonBiomes,
            dungeons.GroupBy(item => item.GameId, StringComparer.Ordinal)
                .ToDictionary(group => group.Key, group => group.Count(), StringComparer.Ordinal));
        AssertValidLocations(fastTravel.Concat(dungeons).ToArray());
    }

    internal static void AssertCompleteNavigationCounts(
        IReadOnlyCollection<WorldCatalogueLocation> fastTravel,
        IReadOnlyCollection<WorldCatalogueLocation> dungeons)
    {
        RequireCount("watchtowers", 22, fastTravel.Count(item => item.Category == "watchtowers"));
        RequireCount("waypoints", 152, fastTravel.Count(item => item.Category == "waypoints"));
        RequireCount("complete dungeon entrances", 170, dungeons.Count);
        AssertHistogram(
            "complete dungeon biomes",
            ExpectedCompleteDungeonBiomes,
            dungeons.GroupBy(item => item.GameId, StringComparer.Ordinal)
                .ToDictionary(group => group.Key, group => group.Count(), StringComparer.Ordinal));
        AssertValidLocations(fastTravel.Concat(dungeons).ToArray());
    }

    internal static void AssertWorldPartitionCounts(
        IReadOnlyCollection<WorldCatalogueLocation> effigies,
        IReadOnlyCollection<WorldCatalogueLocation> journals,
        int persistentJournalCount,
        IReadOnlyCollection<WorldCatalogueLocation> shrinePickups,
        int persistentShrineCount,
        IReadOnlyCollection<WorldCatalogueLocation> npcs,
        JObject pickupRows)
    {
        RequireCount("Effigies", 407, effigies.Count);
        AssertHistogram(
            "Effigy classes",
            ExpectedEffigyClasses,
            effigies.GroupBy(item => item.ClassName!, StringComparer.Ordinal)
                .ToDictionary(group => group.Key, group => group.Count(), StringComparer.Ordinal));
        RequireCount("persistent Journals", 15, persistentJournalCount);
        RequireCount("generated Journals", 49, journals.Count - persistentJournalCount);
        RequireCount("Journals", 64, journals.Count);
        RequireCount("persistent ancient-shrine pickups", 2, persistentShrineCount);
        RequireCount("generated ancient-shrine pickups", 104, shrinePickups.Count - persistentShrineCount);
        RequireCount("ancient-shrine pickups", 106, shrinePickups.Count);
        var unlocalizedRewardIds = shrinePickups
            .SelectMany(item => item.Rewards ?? [])
            .Where(item => item.NameSource != "localized")
            .Select(item => item.ItemId)
            .Distinct(StringComparer.Ordinal)
            .OrderBy(item => item, StringComparer.Ordinal)
            .ToArray();
        if (unlocalizedRewardIds.Length != 1 ||
            unlocalizedRewardIds[0] != "Blueprint_Accessory_Avoid_1_fix")
        {
            throw new InvalidOperationException(
                "Expected Blueprint_Accessory_Avoid_1_fix to be the sole Shrine reward without a localized item-name row; " +
                $"found {string.Join(", ", unlocalizedRewardIds)}.");
        }
        RequireCount("item-pickup DataTable rows", 107, pickupRows.Properties().Count());
        var placedPickupIds = shrinePickups.Select(item => item.GameId).ToHashSet(StringComparer.Ordinal);
        var unusedPickupIds = pickupRows.Properties()
            .Select(item => item.Name)
            .Where(item => !placedPickupIds.Contains(item))
            .OrderBy(item => item, StringComparer.Ordinal)
            .ToArray();
        if (unusedPickupIds.Length != 1 || unusedPickupIds[0] != "Test_GrassLand01")
        {
            throw new InvalidOperationException(
                $"Expected Test_GrassLand01 to be the sole unplaced item-pickup row; found {string.Join(", ", unusedPickupIds)}.");
        }
        RequireCount("game-derived NPC locations", 90, npcs.Count);
        RequireCount("game-derived NPC identities", 81, npcs.Select(item => item.GameId).Distinct(StringComparer.Ordinal).Count());
        AssertHistogram(
            "NPC categories",
            new Dictionary<string, int>(StringComparer.Ordinal)
            {
                ["Black Market"] = 4,
                ["Dog Coin"] = 4,
                ["General"] = 82
            },
            npcs.GroupBy(item => item.Detail!, StringComparer.Ordinal)
                .ToDictionary(group => group.Key, group => group.Count(), StringComparer.Ordinal));
        AssertValidLocations(effigies.Concat(journals).Concat(shrinePickups).Concat(npcs).ToArray());
    }

    internal static void AssertValidLocations(IReadOnlyCollection<WorldCatalogueLocation> locations)
    {
        var duplicateIds = locations
            .GroupBy(item => item.Id, StringComparer.Ordinal)
            .Where(group => group.Count() > 1)
            .Select(group => group.Key)
            .OrderBy(item => item, StringComparer.Ordinal)
            .ToArray();
        if (duplicateIds.Length != 0)
        {
            throw new InvalidOperationException(
                $"Duplicate generated world-catalogue IDs: {string.Join(", ", duplicateIds)}.");
        }

        foreach (var location in locations)
        {
            if (string.IsNullOrWhiteSpace(location.Id) ||
                string.IsNullOrWhiteSpace(location.Category) ||
                string.IsNullOrWhiteSpace(location.GameId) ||
                string.IsNullOrWhiteSpace(location.Name) ||
                !double.IsFinite(location.X) ||
                !double.IsFinite(location.Y) ||
                !double.IsFinite(location.Z))
            {
                throw new InvalidOperationException($"World-catalogue location {location.Id} is incomplete.");
            }
            if (location.InstanceId != null)
            {
                _ = RequireInstanceId(new JValue(location.InstanceId), $"{location.Id}.InstanceId");
            }
        }

        var duplicateInstanceIds = locations
            .Where(item => item.InstanceId != null)
            .GroupBy(item => item.InstanceId, StringComparer.OrdinalIgnoreCase)
            .Where(group => group.Count() > 1)
            .Select(group => group.Key)
            .OrderBy(item => item, StringComparer.Ordinal)
            .ToArray();
        if (duplicateInstanceIds.Length != 0)
        {
            throw new InvalidOperationException(
                $"Duplicate world-catalogue instance IDs: {string.Join(", ", duplicateInstanceIds)}.");
        }
    }

    private static WorldCatalogueLocation ShapeBossTableLocation(
        string rowName,
        JObject row,
        string category,
        string spawnerId,
        string name,
        string detail,
        string? stateKey,
        string iconSource)
    {
        var context = $"{BossSpawnerPackage}[{rowName}]";
        var sourceLocation = LandmarkShaper.RequireObject(row["Location"], $"{context}.Location");
        var x = LandmarkShaper.RequireFiniteDouble(sourceLocation["X"], $"{context}.Location.X");
        var y = LandmarkShaper.RequireFiniteDouble(sourceLocation["Y"], $"{context}.Location.Y");
        var z = LandmarkShaper.RequireFiniteDouble(sourceLocation["Z"], $"{context}.Location.Z");
        var level = LandmarkShaper.RequireInt(row["Level"], $"{context}.Level");
        var coordinateId = string.Create(CultureInfo.InvariantCulture, $"{x:F2}:{y:F2}:{z:F2}");
        return new WorldCatalogueLocation(
            $"{(category == "bounties" ? "bounty" : "oil-rig")}:{spawnerId}:{coordinateId}",
            category,
            spawnerId,
            name,
            x,
            y,
            z)
        {
            Detail = detail,
            Level = level,
            StateKey = stateKey,
            IconSource = iconSource,
            SourcePackage = GameAssetReader.ObjectPath(BossSpawnerPackage),
            SourceObject = rowName
        };
    }

    private static WorldCatalogueLocation WithActorSource(
        WorldCatalogueLocation location,
        PlacedActor actor) =>
        location with
        {
            SourcePackage = actor.SourcePackage,
            SourceObject = actor.ActorName
        };

    private static bool EquivalentExceptSourceObject(
        WorldCatalogueLocation left,
        WorldCatalogueLocation right) =>
        left with { SourceObject = null } == right with { SourceObject = null };

    private static CatalogueReward ShapeReward(
        string itemId,
        int count,
        JObject itemRows,
        JObject itemNameRows,
        JObject itemIconRows)
    {
        var item = LandmarkShaper.RequireRow(itemRows, itemId, ItemDataPackage);
        var (name, nameSource) = ResolveItemName(itemNameRows, itemId);
        var iconName = OptionalName(item["IconName"], $"{ItemDataPackage}[{itemId}].IconName") ?? itemId;
        var iconRow = LandmarkShaper.RequireRow(itemIconRows, iconName, ItemIconPackage);
        var icon = LandmarkShaper.RequireObject(iconRow["Icon"], $"{ItemIconPackage}[{iconName}].Icon");
        var iconSource = LandmarkShaper.RequireString(
            icon["AssetPathName"],
            $"{ItemIconPackage}[{iconName}].Icon.AssetPathName");
        return new CatalogueReward(itemId, name, count, iconSource, nameSource);
    }

    private static (string Name, string Source) ResolveItemName(JObject itemNameRows, string itemId)
    {
        var candidates = new[]
        {
            $"ITEM_NAME_{itemId}",
            $"ITEM_NAME_{itemId}_TextData"
        };
        var matches = candidates
            .Select(key => (key, row: FindRow(itemNameRows, key, StringComparison.Ordinal)))
            .Where(item => item.row != null)
            .ToArray();
        if (matches.Length > 1)
        {
            throw new InvalidOperationException(
                $"Expected at most one item-name row for {itemId} in {ItemNamePackage}, found {matches.Length}.");
        }
        return matches.Length == 0
            ? (itemId, "game-id (no localized row)")
            : (
                LandmarkShaper.ResolveLocalizedText(matches[0].row!, $"{ItemNamePackage}[{matches[0].key}]"),
                "localized");
    }

    private static (string Title, string? Preview) JournalTitleAndPreview(string fullText)
    {
        var normalized = fullText.Replace("\r\n", "\n", StringComparison.Ordinal);
        var separator = normalized.IndexOf("\n\n", StringComparison.Ordinal);
        var title = (separator < 0 ? normalized : normalized[..separator]).Trim();
        if (string.IsNullOrWhiteSpace(title))
        {
            throw new InvalidOperationException("Journal localized text has no title.");
        }
        if (separator < 0)
        {
            return (title, null);
        }
        var body = Whitespace().Replace(normalized[(separator + 2)..].Replace("|", string.Empty, StringComparison.Ordinal), " ").Trim();
        if (body.Length == 0)
        {
            return (title, null);
        }
        var preview = body.Length <= 160 ? body : body[..160].TrimEnd() + "...";
        return (title, preview);
    }

    private static string RequireName(JToken? token, string context)
    {
        var value = OptionalName(token, context);
        return value ?? throw new InvalidOperationException($"Expected a non-empty name at {context}.");
    }

    private static string? OptionalName(JToken? token, string context)
    {
        if (token == null || token.Type == JTokenType.Null)
        {
            return null;
        }
        string? value = token.Type == JTokenType.String
            ? token.Value<string>()
            : token is JObject obj && obj["Key"]?.Type == JTokenType.String
                ? obj["Key"]!.Value<string>()
                : null;
        if (value == null)
        {
            throw new InvalidOperationException($"Expected a string or row-handle Key at {context}.");
        }
        value = value.Trim();
        return value.Length == 0 || LandmarkShaper.NormalizeEnum(value) == "None" ? null : value;
    }

    private static int? OptionalInt(JToken? token, string context)
    {
        if (token == null || token.Type == JTokenType.Null)
        {
            return null;
        }
        return LandmarkShaper.RequireInt(token, context);
    }

    private static InstanceId RequireInstanceId(JToken? token, string context)
    {
        var raw = RequireName(token, context).ToUpperInvariant();
        var normalized = raw.Replace("-", string.Empty, StringComparison.Ordinal);
        if (normalized.Length != 32 || normalized.Any(character => !Uri.IsHexDigit(character)))
        {
            throw new InvalidOperationException($"Expected a non-zero 32-hex-character instance GUID at {context}.");
        }
        if (normalized.All(character => character == '0'))
        {
            throw new InvalidOperationException($"Expected a non-zero 32-hex-character instance GUID at {context}.");
        }
        return new InstanceId(raw, normalized);
    }

    private static JObject RequireRowIgnoreCase(JObject rows, string key, string packagePath)
    {
        var matches = rows.Properties()
            .Where(property => string.Equals(property.Name, key, StringComparison.OrdinalIgnoreCase))
            .ToArray();
        if (matches.Length != 1)
        {
            throw new InvalidOperationException(
                $"{packagePath} expected one case-insensitive row named {key}, found {matches.Length}.");
        }
        return LandmarkShaper.RequireObject(matches[0].Value, $"{packagePath}[{matches[0].Name}]");
    }

    private static JObject? FindRow(JObject rows, string key, StringComparison comparison)
    {
        var matches = rows.Properties()
            .Where(property => string.Equals(property.Name, key, comparison))
            .ToArray();
        if (matches.Length > 1)
        {
            throw new InvalidOperationException($"Multiple rows match {key}.");
        }
        return matches.Length == 0 ? null : LandmarkShaper.RequireObject(matches[0].Value, key);
    }

    private static string ReadableIdentifier(string value)
    {
        var spaced = IdentifierBoundary().Replace(value.Replace("_", " ", StringComparison.Ordinal), " $1");
        return Whitespace().Replace(spaced, " ").Trim();
    }

    private static void RequireCount(string description, int expected, int actual)
    {
        if (actual != expected)
        {
            throw new InvalidOperationException($"Expected {expected} {description}, extracted {actual}.");
        }
    }

    private static void AssertHistogram(
        string description,
        IReadOnlyDictionary<string, int> expected,
        IReadOnlyDictionary<string, int> actual)
    {
        if (expected.Count == actual.Count &&
            expected.All(item => actual.TryGetValue(item.Key, out var count) && count == item.Value))
        {
            return;
        }
        static string Format(IReadOnlyDictionary<string, int> values) => string.Join(
            ", ",
            values.OrderBy(item => item.Key, StringComparer.Ordinal).Select(item => $"{item.Key}={item.Value}"));
        throw new InvalidOperationException(
            $"Unexpected {description}. Expected [{Format(expected)}], extracted [{Format(actual)}].");
    }

    [GeneratedRegex(@"(?<!^)([A-Z])")]
    private static partial Regex IdentifierBoundary();

    [GeneratedRegex(@"\s+")]
    private static partial Regex Whitespace();

    private sealed record InstanceId(string Raw, string Normalized);
}
