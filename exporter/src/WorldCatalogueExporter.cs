using System.Text.Json;
using System.Text.Json.Serialization;
using CUE4Parse.FileProvider;
using CUE4Parse.UE4.Assets.Exports;
using Newtonsoft.Json.Linq;

internal static class WorldCatalogueExporter
{
    private const int CatalogueSchemaVersion = 1;
    private const int DatasetSchemaVersion = 1;
    private const int ExpectedWorldPartitionPackages = 9_977;

    private const string BossSpawnerPackage = "Pal/Content/Pal/DataTable/UI/DT_BossSpawnerLoactionData";
    private const string HumanParameterPackage = "Pal/Content/Pal/DataTable/Character/DT_PalHumanParameter";
    private const string HumanNamePackage = "Pal/Content/L10N/en/Pal/DataTable/Text/DT_HumanNameText_Common";
    private const string RegionNamePackage = "Pal/Content/L10N/en/Pal/DataTable/Text/DT_WorldMap_Common_Text_Common";
    private const string BossIconPackage = "Pal/Content/Pal/DataTable/Character/DT_PalBossNPCIcon";
    private const string MainWorldPackage = "Pal/Content/Pal/Maps/MainWorld_5/PL_MainWorld5";
    private const string WorldPartitionPrefix = MainWorldPackage + "/_Generated_/";
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

    private static readonly HashSet<string> FastTravelClasses =
        new(StringComparer.Ordinal)
        {
            "BP_LevelObject_UnlockMapPoint_C",
            "BP_LevelObject_TowerFastTravelPoint_C"
        };

    private static readonly HashSet<string> NpcSpawnerClasses =
        new(StringComparer.Ordinal)
        {
            "BP_MonoNPCSpawner_C",
            "BP_MonoNPCSpawner_Unique_C",
            "BP_MonoNPCSpawner_MedalTrader_C"
        };

    private static readonly JsonSerializerOptions JsonOptions = new()
    {
        WriteIndented = true,
        PropertyNamingPolicy = JsonNamingPolicy.CamelCase,
        DefaultIgnoreCondition = JsonIgnoreCondition.WhenWritingNull
    };

    internal static void Export(
        DefaultFileProvider provider,
        string outputDirectory,
        string gameVersion,
        string generator,
        string decoder,
        SourceFile mappings,
        SourceFile[] paks,
        LandmarkExtraction shared)
    {
        Console.WriteLine("Extracting expanded world catalogue from game data...");

        var humanRows = GameAssetReader.LoadRows(provider, HumanParameterPackage);
        var humanNameRows = GameAssetReader.LoadRows(provider, HumanNamePackage);
        var regionNameRows = GameAssetReader.LoadRows(provider, RegionNamePackage);
        var bossIconRows = GameAssetReader.LoadRows(provider, BossIconPackage);
        var respawnNameRows = GameAssetReader.LoadRows(provider, RespawnNamePackage);
        var palNameRows = GameAssetReader.LoadRows(provider, PalNamePackage);
        var noteDescRows = GameAssetReader.LoadRows(provider, NoteDescPackage);
        var noteTextureRows = GameAssetReader.LoadRows(provider, NoteTexturePackage);
        var pickupRows = GameAssetReader.LoadRows(provider, ItemPickupPackage);
        var itemRows = GameAssetReader.LoadRows(provider, ItemDataPackage);
        var itemIconRows = GameAssetReader.LoadRows(provider, ItemIconPackage);
        var itemNameRows = GameAssetReader.LoadRows(provider, ItemNamePackage);
        var uniqueNpcRows = GameAssetReader.LoadRows(provider, UniqueNpcPackage);
        var uniqueNpcNameRows = GameAssetReader.LoadRows(provider, UniqueNpcNamePackage);

        var encounterAdditions = WorldCatalogueShaper.ShapeBossAdditions(
            shared.BossRows,
            humanRows,
            humanNameRows,
            regionNameRows,
            bossIconRows);

        var fastTravel = new List<WorldCatalogueLocation>();
        var dungeons = new List<WorldCatalogueLocation>();
        var effigies = new List<WorldCatalogueLocation>();
        var journals = new List<WorldCatalogueLocation>();
        var shrinePickups = new List<WorldCatalogueLocation>();
        var npcs = new List<WorldCatalogueLocation>();
        var npcExclusions = new Dictionary<string, int>(StringComparer.Ordinal);

        ScanExports(
            shared.MainWorldExports,
            GameAssetReader.ObjectPath(MainWorldPackage),
            respawnNameRows,
            palNameRows,
            noteDescRows,
            noteTextureRows,
            pickupRows,
            itemRows,
            itemNameRows,
            itemIconRows,
            uniqueNpcRows,
            uniqueNpcNameRows,
            fastTravel,
            dungeons,
            effigies,
            journals,
            shrinePickups,
            npcs,
            npcExclusions);
        var persistentJournalCount = journals.Count;
        var persistentShrineCount = shrinePickups.Count;

        WorldCatalogueShaper.AssertPersistentCounts(fastTravel, dungeons);

        var cellPaths = provider.Files.Keys
            .Select(path => path.ToString())
            .Where(path =>
                path.StartsWith(WorldPartitionPrefix, StringComparison.OrdinalIgnoreCase) &&
                path.EndsWith(".umap", StringComparison.OrdinalIgnoreCase))
            .Distinct(StringComparer.OrdinalIgnoreCase)
            .OrderBy(path => path, StringComparer.Ordinal)
            .ToArray();
        if (cellPaths.Length != ExpectedWorldPartitionPackages)
        {
            throw new InvalidOperationException(
                $"Expected {ExpectedWorldPartitionPackages} World Partition packages under {WorldPartitionPrefix}, found {cellPaths.Length}.");
        }

        Console.WriteLine($"Scanning {cellPaths.Length} World Partition packages once for collectibles and fixed NPC spawners...");
        for (var index = 0; index < cellPaths.Length; index++)
        {
            var cellPath = cellPaths[index];
            UObject[] exports;
            try
            {
                exports = provider.LoadPackage(cellPath).GetExports().ToArray();
            }
            catch (Exception exception)
            {
                throw new InvalidOperationException(
                    $"Failed to load World Partition package {cellPath}; refusing to emit a partial catalogue.",
                    exception);
            }

            ScanExports(
                exports,
                cellPath,
                respawnNameRows,
                palNameRows,
                noteDescRows,
                noteTextureRows,
                pickupRows,
                itemRows,
                itemNameRows,
                itemIconRows,
                uniqueNpcRows,
                uniqueNpcNameRows,
                fastTravel,
                dungeons,
                effigies,
                journals,
                shrinePickups,
                npcs,
                npcExclusions);
            if ((index + 1) % 500 == 0 || index + 1 == cellPaths.Length)
            {
                Console.WriteLine($"...scanned {index + 1}/{cellPaths.Length} World Partition packages");
            }
        }

        WorldCatalogueShaper.AssertWorldPartitionCounts(
            effigies,
            journals,
            persistentJournalCount,
            shrinePickups,
            persistentShrineCount,
            npcs,
            pickupRows);
        WorldCatalogueShaper.AssertCompleteNavigationCounts(fastTravel, dungeons);
        AssertNpcExclusions(npcExclusions);

        var navigation = fastTravel.Concat(dungeons).ToArray();
        var collectibles = effigies.Concat(journals).Concat(shrinePickups).ToArray();
        var allLocations = encounterAdditions
            .Concat(navigation)
            .Concat(collectibles)
            .Concat(npcs)
            .ToArray();
        WorldCatalogueShaper.AssertValidLocations(allLocations);

        var catalogueDirectory = Path.Combine(outputDirectory, "catalogue");
        Directory.CreateDirectory(catalogueDirectory);
        var datasets = new[]
        {
            WriteDataset(catalogueDirectory, "encounter-additions", encounterAdditions),
            WriteDataset(catalogueDirectory, "navigation", navigation),
            WriteDataset(catalogueDirectory, "collectibles", collectibles),
            WriteDataset(catalogueDirectory, "npc-locations", npcs)
        };
        var manifest = new WorldCatalogueManifest(
            CatalogueSchemaVersion,
            gameVersion,
            generator,
            decoder,
            mappings,
            paks.OrderBy(item => item.File, StringComparer.Ordinal).ToArray(),
            Sources,
            new WorldCatalogueScan(
                GameAssetReader.ObjectPath(MainWorldPackage),
                WorldPartitionPrefix + "*.umap",
                cellPaths.Length,
                npcExclusions
                    .OrderBy(item => item.Key, StringComparer.Ordinal)
                    .Select(item => new WorldCatalogueExclusion(item.Key, item.Value))
                    .ToArray()),
            datasets);
        var manifestPath = Path.Combine(catalogueDirectory, "manifest.json");
        WriteJson(manifestPath, manifest);

        Console.WriteLine(
            "Staged expanded catalogue " +
            $"({encounterAdditions.Length} encounter additions, {navigation.Length} navigation, " +
            $"{collectibles.Length} collectibles, {npcs.Count} fixed NPC locations; {allLocations.Length} total new records)");
    }

    private static void ScanExports(
        IEnumerable<UObject> exports,
        string sourcePackage,
        JObject respawnNameRows,
        JObject palNameRows,
        JObject noteDescRows,
        JObject noteTextureRows,
        JObject pickupRows,
        JObject itemRows,
        JObject itemNameRows,
        JObject itemIconRows,
        JObject uniqueNpcRows,
        JObject uniqueNpcNameRows,
        ICollection<WorldCatalogueLocation> fastTravel,
        ICollection<WorldCatalogueLocation> dungeons,
        ICollection<WorldCatalogueLocation> effigies,
        ICollection<WorldCatalogueLocation> journals,
        ICollection<WorldCatalogueLocation> shrinePickups,
        ICollection<WorldCatalogueLocation> npcs,
        IDictionary<string, int> npcExclusions)
    {
        foreach (var export in exports)
        {
            var className = export.Class?.Name.ToString() ?? string.Empty;
            var relevant =
                FastTravelClasses.Contains(className) ||
                className.StartsWith("BP_DungeonPortalMarker_", StringComparison.Ordinal) ||
                className.StartsWith("BP_LevelObject_Relic", StringComparison.Ordinal) ||
                className == "BP_LevelObject_Note_C" ||
                className == "BP_LevelObject_ItemPickupTower_C" ||
                NpcSpawnerClasses.Contains(className);
            if (!relevant)
            {
                continue;
            }

            if (NpcSpawnerClasses.Contains(className) && IsBlueprintTemplate(export.Name.ToString()))
            {
                IncrementExclusion(npcExclusions, "Blueprint template export (not a placed actor)");
                continue;
            }

            var actor = GameAssetReader.ReadPlacement(export, sourcePackage);
            if (FastTravelClasses.Contains(className))
            {
                fastTravel.Add(WorldCatalogueShaper.ShapeFastTravel(actor, respawnNameRows));
            }
            else if (className.StartsWith("BP_DungeonPortalMarker_", StringComparison.Ordinal))
            {
                dungeons.Add(WorldCatalogueShaper.ShapeDungeon(actor));
            }
            else if (className.StartsWith("BP_LevelObject_Relic", StringComparison.Ordinal))
            {
                effigies.Add(WorldCatalogueShaper.ShapeEffigy(actor, palNameRows));
            }
            else if (className == "BP_LevelObject_Note_C")
            {
                journals.Add(WorldCatalogueShaper.ShapeJournal(actor, noteDescRows, noteTextureRows));
            }
            else if (className == "BP_LevelObject_ItemPickupTower_C")
            {
                shrinePickups.Add(WorldCatalogueShaper.ShapeAncientShrinePickup(
                    actor,
                    pickupRows,
                    itemRows,
                    itemNameRows,
                    itemIconRows));
            }
            else
            {
                var npc = WorldCatalogueShaper.ShapeNpc(
                    actor,
                    uniqueNpcRows,
                    uniqueNpcNameRows,
                    out var exclusionReason);
                if (npc != null)
                {
                    npcs.Add(npc);
                }
                else
                {
                    if (string.IsNullOrWhiteSpace(exclusionReason))
                    {
                        throw new InvalidOperationException(
                            $"NPC shaper omitted {actor.ActorName} without an exclusion reason.");
                    }
                    IncrementExclusion(npcExclusions, exclusionReason);
                }
            }
        }
    }

    private static bool IsBlueprintTemplate(string objectName) =>
        objectName.StartsWith("Default__", StringComparison.Ordinal) ||
        objectName.Contains("_GEN_VARIABLE_", StringComparison.Ordinal);

    private static void IncrementExclusion(IDictionary<string, int> exclusions, string reason) =>
        exclusions[reason] = exclusions.TryGetValue(reason, out var count) ? count + 1 : 1;

    private static void AssertNpcExclusions(IReadOnlyDictionary<string, int> exclusions)
    {
        var expected = new Dictionary<string, int>(StringComparer.Ordinal)
        {
            ["Blueprint template export (not a placed actor)"] = 25,
            ["non-character reward or emote spawner"] = 30,
            ["unconfigured spawner (no UniqueName)"] = 25
        };
        if (expected.Count == exclusions.Count &&
            expected.All(item => exclusions.TryGetValue(item.Key, out var count) && count == item.Value))
        {
            return;
        }
        static string Format(IEnumerable<KeyValuePair<string, int>> values) => string.Join(
            ", ",
            values.OrderBy(item => item.Key, StringComparer.Ordinal).Select(item => $"{item.Key}={item.Value}"));
        throw new InvalidOperationException(
            $"Unexpected NPC-spawner exclusions. Expected [{Format(expected)}], extracted [{Format(exclusions)}].");
    }

    private static WorldCatalogueDatasetReference WriteDataset(
        string catalogueDirectory,
        string id,
        IEnumerable<WorldCatalogueLocation> locations)
    {
        var ordered = locations
            .OrderBy(item => item.Category, StringComparer.Ordinal)
            .ThenBy(item => item.Id, StringComparer.Ordinal)
            .ToArray();
        WorldCatalogueShaper.AssertValidLocations(ordered);
        var file = id + ".json";
        var destination = Path.Combine(catalogueDirectory, file);
        WriteJson(destination, new WorldCatalogueDataset(DatasetSchemaVersion, id, ordered));
        return new WorldCatalogueDatasetReference(id, file, ordered.Length, SourceSnapshots.HashFile(destination));
    }

    private static void WriteJson<T>(string destination, T value) =>
        File.WriteAllText(destination, JsonSerializer.Serialize(value, JsonOptions) + Environment.NewLine);

    private static readonly LandmarkSource[] Sources =
    [
        new(GameVersionExtractor.SourcePath, "Palworld ProjectVersion used for catalogue compatibility"),
        new(GameAssetReader.ObjectPath(BossSpawnerPackage), "Bounty and oil-rig IDs, levels, and coordinates"),
        new(GameAssetReader.ObjectPath(HumanParameterPackage), "Bounty human-name text IDs"),
        new(GameAssetReader.ObjectPath(HumanNamePackage), "English Bounty display names"),
        new(GameAssetReader.ObjectPath(RegionNamePackage), "English oil-rig display names"),
        new(GameAssetReader.ObjectPath(BossIconPackage), "Bounty portrait texture references"),
        new(GameAssetReader.ObjectPath(MainWorldPackage), "Persistent navigation, dungeon, Journal, Shrine, and NPC placements"),
        new(GameAssetReader.ObjectPath(RespawnNamePackage), "English watchtower and waypoint display names"),
        new(WorldPartitionPrefix + "*.umap", "Generated Effigy, Journal, Shrine, and fixed NPC placements"),
        new(GameAssetReader.ObjectPath(PalNamePackage), "English Effigy Pal names"),
        new(GameAssetReader.ObjectPath(NoteDescPackage), "English Journal titles and preview text"),
        new(GameAssetReader.ObjectPath(NoteTexturePackage), "Journal texture references"),
        new(GameAssetReader.ObjectPath(ItemPickupPackage), "Ancient Shrine reward item IDs and counts"),
        new(GameAssetReader.ObjectPath(ItemDataPackage), "Reward icon IDs"),
        new(GameAssetReader.ObjectPath(ItemNamePackage), "English reward item names"),
        new(GameAssetReader.ObjectPath(ItemIconPackage), "Reward texture references"),
        new(GameAssetReader.ObjectPath(UniqueNpcPackage), "Fixed NPC identity and name-text IDs"),
        new(GameAssetReader.ObjectPath(UniqueNpcNamePackage), "English fixed NPC display names")
    ];
}
