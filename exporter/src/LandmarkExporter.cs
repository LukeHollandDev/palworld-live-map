using System.Text.Json;
using System.Text.Json.Serialization;
using CUE4Parse.FileProvider;
using CUE4Parse.UE4.Assets.Exports;
using CUE4Parse.UE4.Objects.Core.Math;
using Newtonsoft.Json;
using Newtonsoft.Json.Linq;

internal static class LandmarkExporter
{
    private const int ExpectedAlphaCount = 90;
    private const int ExpectedTowerCount = 9;

    private const string BossSpawnerPackage = "Pal/Content/Pal/DataTable/UI/DT_BossSpawnerLoactionData";
    private const string MonsterParameterPackage = "Pal/Content/Pal/DataTable/Character/DT_PalMonsterParameter";
    private const string PalNamePackage = "Pal/Content/L10N/en/Pal/DataTable/Text/DT_PalNameText_Common";
    private const string RegionNamePackage = "Pal/Content/L10N/en/Pal/DataTable/Text/DT_WorldMap_Common_Text_Common";
    private const string BossManagerPackage = "Pal/Content/Pal/Blueprint/System/BP_PalBossBattleManager";
    private const string MainWorldPackage = "Pal/Content/Pal/Maps/MainWorld_5/PL_MainWorld5";

    private static readonly LandmarkSource[] Sources =
    [
        new(GameVersionExtractor.SourcePath, "Palworld ProjectVersion used for catalogue compatibility"),
        new(ObjectPath(BossSpawnerPackage), "Field Alpha spawner IDs, levels, and coordinates"),
        new(ObjectPath(MonsterParameterPackage), "Alpha and tower Pal names and elements"),
        new(ObjectPath(PalNamePackage), "English encounter display names"),
        new(ObjectPath(RegionNamePackage), "English tower display names"),
        new($"{BossManagerPackage}.Default__BP_PalBossBattleManager_C", "Tower BossType to normal-difficulty Pal and level"),
        new(ObjectPath(MainWorldPackage), "Placed tower actors and root-component coordinates")
    ];

    public static void Export(
        DefaultFileProvider provider,
        string outputDirectory,
        string gameVersion,
        string generator,
        string decoder,
        SourceFile mappings,
        SourceFile[] paks)
    {
        Console.WriteLine("Extracting encounter landmarks from game data...");

        var monsterRows = LoadRows(provider, MonsterParameterPackage);
        var palNameRows = LoadRows(provider, PalNamePackage);
        var alphaLocations = ExtractAlphas(provider, monsterRows, palNameRows);
        var towerLocations = ExtractTowers(provider, monsterRows, palNameRows);

        var locations = alphaLocations
            .Concat(towerLocations)
            .OrderBy(item => item.Kind, StringComparer.Ordinal)
            .ThenBy(item => item.Id, StringComparer.Ordinal)
            .ToArray();
        LandmarkShaper.AssertUniqueLocationIds(locations);

        var manifest = new LandmarkManifest(
            2,
            gameVersion,
            generator,
            decoder,
            mappings,
            paks.OrderBy(item => item.File, StringComparer.Ordinal).ToArray(),
            Sources,
            locations);
        var destination = Path.Combine(outputDirectory, "manifest.json");
        File.WriteAllText(destination, System.Text.Json.JsonSerializer.Serialize(manifest, new JsonSerializerOptions
        {
            WriteIndented = true,
            PropertyNamingPolicy = JsonNamingPolicy.CamelCase,
            DefaultIgnoreCondition = JsonIgnoreCondition.WhenWritingNull
        }) + Environment.NewLine);
        Console.WriteLine($"Staged landmark manifest.json ({alphaLocations.Count} Alphas, {towerLocations.Count} towers)");
    }

    private static List<LandmarkLocation> ExtractAlphas(
        DefaultFileProvider provider,
        JObject monsterRows,
        JObject palNameRows)
    {
        var bossRows = LoadRows(provider, BossSpawnerPackage);
        var locations = new List<LandmarkLocation>(ExpectedAlphaCount);
        var spawnerIds = new List<string>(ExpectedAlphaCount);

        foreach (var property in bossRows.Properties().OrderBy(item => item.Name, StringComparer.Ordinal))
        {
            var context = $"boss-spawner row {property.Name}";
            var row = LandmarkShaper.RequireObject(property.Value, context);
            var shaped = LandmarkShaper.ShapeAlpha(property.Name, row, monsterRows, palNameRows);
            if (shaped == null)
            {
                continue;
            }

            locations.Add(shaped.Location);
            spawnerIds.Add(shaped.SpawnerId);
        }

        if (locations.Count != ExpectedAlphaCount)
        {
            throw new InvalidOperationException($"Expected {ExpectedAlphaCount} Alpha locations, extracted {locations.Count}.");
        }

        var repeatedSpawnerIds = spawnerIds
            .GroupBy(item => item, StringComparer.Ordinal)
            .Where(group => group.Count() > 1)
            .OrderBy(group => group.Key, StringComparer.Ordinal)
            .ToArray();
        if (repeatedSpawnerIds.Length != 1 || repeatedSpawnerIds[0].Count() != 2)
        {
            var summary = repeatedSpawnerIds.Length == 0
                ? "none"
                : string.Join(", ", repeatedSpawnerIds.Select(group => $"{group.Key} ({group.Count()})"));
            throw new InvalidOperationException($"Expected one duplicated Alpha SpawnerID with two locations; found {summary}.");
        }

        return locations;
    }

    private static List<LandmarkLocation> ExtractTowers(
        DefaultFileProvider provider,
        JObject monsterRows,
        JObject palNameRows)
    {
        var regionRows = LoadRows(provider, RegionNamePackage);
        var bossParameters = LoadBossParameters(provider);
        var levelObjects = provider.LoadPackage(MainWorldPackage).GetExports().ToArray();
        var towerActors = levelObjects
            .Where(item => item.Class?.Name.ToString() is "BP_PalBossTower_C" or "BP_PalBossTower_LastBoss_C")
            .ToArray();

        var regularCount = towerActors.Count(item => item.Class?.Name.ToString() == "BP_PalBossTower_C");
        var lastBossCount = towerActors.Count(item => item.Class?.Name.ToString() == "BP_PalBossTower_LastBoss_C");
        if (towerActors.Length != ExpectedTowerCount || regularCount != 8 || lastBossCount != 1)
        {
            throw new InvalidOperationException(
                $"Expected 8 BP_PalBossTower_C actors and 1 BP_PalBossTower_LastBoss_C actor; " +
                $"found {regularCount} and {lastBossCount}.");
        }

        var locations = new List<LandmarkLocation>(ExpectedTowerCount);
        var seenBossTypes = new HashSet<string>(StringComparer.Ordinal);
        foreach (var actor in towerActors.OrderBy(item => item.Name, StringComparer.Ordinal))
        {
            var context = $"tower actor {actor.Name}";
            var actorJson = SerializeObject(actor);
            var properties = LandmarkShaper.RequireObject(actorJson["Properties"], $"{context}.Properties");
            var bossType = LandmarkShaper.NormalizeEnum(LandmarkShaper.RequireString(properties["BossType"], $"{context}.BossType"));
            if (!seenBossTypes.Add(bossType))
            {
                throw new InvalidOperationException($"Duplicate placed tower BossType {bossType}.");
            }
            if (!actor.TryGetValue<UObject>(out var rootComponent, "RootComponent"))
            {
                throw new InvalidOperationException($"{context} has no loadable RootComponent.");
            }
            if (!rootComponent.TryGetValue<FVector>(out var position, "RelativeLocation"))
            {
                throw new InvalidOperationException($"{context} RootComponent has no RelativeLocation.");
            }

            locations.Add(LandmarkShaper.ShapeTower(
                new TowerPlacement(actor.Name.ToString(), bossType, position.X, position.Y),
                monsterRows,
                palNameRows,
                regionRows,
                bossParameters));
        }

        if (locations.Count != ExpectedTowerCount || seenBossTypes.Count != ExpectedTowerCount)
        {
            throw new InvalidOperationException(
                $"Expected {ExpectedTowerCount} unique tower locations, extracted {locations.Count} across {seenBossTypes.Count} BossTypes.");
        }
        if (!seenBossTypes.SetEquals(LandmarkShaper.TowerBossTypes))
        {
            var missing = LandmarkShaper.TowerBossTypes.Except(seenBossTypes, StringComparer.Ordinal).OrderBy(item => item, StringComparer.Ordinal);
            throw new InvalidOperationException($"No placed tower actor was found for mapped BossTypes: {string.Join(", ", missing)}.");
        }

        return locations;
    }

    private static Dictionary<string, BossParameter> LoadBossParameters(DefaultFileProvider provider)
    {
        var exports = provider.LoadPackage(BossManagerPackage).GetExports().ToArray();
        var managers = exports.Where(item => item.Name == "Default__BP_PalBossBattleManager_C").ToArray();
        if (managers.Length != 1)
        {
            throw new InvalidOperationException(
                $"Expected one Default__BP_PalBossBattleManager_C export in {BossManagerPackage}, found {managers.Length}.");
        }

        var manager = SerializeObject(managers[0]);
        var properties = LandmarkShaper.RequireObject(manager["Properties"], "Default__BP_PalBossBattleManager_C.Properties");
        var bossInfoMap = LandmarkShaper.RequireArray(properties["BossInfoMap"], "Default__BP_PalBossBattleManager_C.BossInfoMap");
        var result = new Dictionary<string, BossParameter>(StringComparer.Ordinal);
        foreach (var entryToken in bossInfoMap)
        {
            var entry = LandmarkShaper.RequireObject(entryToken, "BossInfoMap entry");
            var bossType = LandmarkShaper.NormalizeEnum(LandmarkShaper.RequireString(entry["Key"], "BossInfoMap entry.Key"));
            var info = LandmarkShaper.RequireObject(entry["Value"], $"BossInfoMap[{bossType}].Value");
            var difficultyMap = LandmarkShaper.RequireArray(info["DifficultyParameter"], $"BossInfoMap[{bossType}].DifficultyParameter");
            var normalEntries = difficultyMap
                .Select(item => LandmarkShaper.RequireObject(item, $"BossInfoMap[{bossType}].DifficultyParameter entry"))
                .Where(item => LandmarkShaper.NormalizeEnum(LandmarkShaper.RequireString(item["Key"], $"BossInfoMap[{bossType}].DifficultyParameter.Key")) == "Normal")
                .ToArray();
            if (normalEntries.Length != 1)
            {
                throw new InvalidOperationException(
                    $"Expected one Normal DifficultyParameter for BossInfoMap[{bossType}], found {normalEntries.Length}.");
            }

            var normal = LandmarkShaper.RequireObject(normalEntries[0]["Value"], $"BossInfoMap[{bossType}].DifficultyParameter[Normal]");
            var palId = LandmarkShaper.RequireObject(normal["PalId"], $"BossInfoMap[{bossType}].DifficultyParameter[Normal].PalId");
            var palKey = LandmarkShaper.RequireString(palId["Key"], $"BossInfoMap[{bossType}].DifficultyParameter[Normal].PalId.Key");
            var level = LandmarkShaper.RequireInt(normal["Level"], $"BossInfoMap[{bossType}].DifficultyParameter[Normal].Level");
            if (!result.TryAdd(bossType, new BossParameter(palKey, level)))
            {
                throw new InvalidOperationException($"BossInfoMap contains duplicate BossType {bossType}.");
            }
        }

        return result;
    }

    private static JObject LoadRows(DefaultFileProvider provider, string packagePath)
    {
        var tables = provider.LoadPackage(packagePath).GetExports()
            .Where(item => (item.Class?.Name.ToString() ?? string.Empty).Contains("DataTable", StringComparison.Ordinal))
            .ToArray();
        if (tables.Length != 1)
        {
            throw new InvalidOperationException($"Expected one DataTable export in {packagePath}, found {tables.Length}.");
        }

        var table = SerializeObject(tables[0]);
        return LandmarkShaper.RequireObject(table["Rows"], $"{packagePath}.Rows");
    }

    private static JObject SerializeObject(UObject value) =>
        JObject.Parse(JsonConvert.SerializeObject(value));

    private static string ObjectPath(string packagePath)
    {
        var name = packagePath[(packagePath.LastIndexOf('/') + 1)..];
        return $"{packagePath}.{name}";
    }

}

internal sealed record LandmarkSource(string Object, string Purpose);

internal sealed record LandmarkLocation(
    string Id,
    string Kind,
    string Name,
    string Detail,
    int? Level,
    double X,
    double Y);

internal sealed record LandmarkManifest(
    int SchemaVersion,
    string GameVersion,
    string Generator,
    string Decoder,
    SourceFile Mappings,
    SourceFile[] Paks,
    LandmarkSource[] Sources,
    LandmarkLocation[] Locations);
