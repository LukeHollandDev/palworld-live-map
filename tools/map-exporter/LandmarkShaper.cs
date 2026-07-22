using System.Globalization;
using Newtonsoft.Json.Linq;

// Converts already-decoded game rows into the stable landmark schema. Keeping
// this layer independent of CUE4Parse makes the joins and fail-closed checks
// executable against small fixtures in CI.
internal static class LandmarkShaper
{
    private const string MonsterParameterPackage = "Pal/Content/Pal/DataTable/Character/DT_PalMonsterParameter";
    private const string PalNamePackage = "Pal/Content/L10N/en/Pal/DataTable/Text/DT_PalNameText_Common";
    private const string RegionNamePackage = "Pal/Content/L10N/en/Pal/DataTable/Text/DT_WorldMap_Common_Text_Common";

    // EPalBossType does not carry the UI region-row spelling. Keep this join
    // explicit and fail closed so a renamed or newly added boss cannot be
    // silently assigned to the wrong map landmark.
    private static readonly IReadOnlyDictionary<string, TowerRegion> TowerRegions =
        new Dictionary<string, TowerRegion>(StringComparer.Ordinal)
        {
            ["GrassBoss"] = new("REGION_Grass_Boss", "REGION_Grass_Boss"),
            ["ForestBoss"] = new("REGION_Forest_Boss", "REGION_Forest_Boss"),
            ["ElectricBoss"] = new("REGION_Volcano_Boss", "REGION_Volcano_Boss"),
            ["DesertBoss"] = new("REGION_Desert_Boss", "REGION_Desert_Boss"),
            ["SnowBoss"] = new("REGION_Frost_Boss", "REGION_Frost_Boss"),
            ["SakurajimaBoss"] = new("REGION_Sakurajima_Boss", "REGION_Sakurajima_Boss"),
            ["VikingBoss"] = new("REGION_Darkisland_Boss", "REGION_Darkisland_Boss"),
            ["SorajimaBoss"] = new("REGION_Skyisland_Boss", "REGION_Skyisland_Boss"),
            ["WorldTreeBoss"] = new("REGION_WorldTree_Boss", "REGION_WorldTree08")
        };

    internal static IEnumerable<string> TowerBossTypes => TowerRegions.Keys;

    internal static ShapedAlpha? ShapeAlpha(
        string rowName,
        JObject row,
        JObject monsterRows,
        JObject palNameRows)
    {
        var context = $"boss-spawner row {rowName}";
        var characterId = RequireString(row["CharacterID"], $"{context}.CharacterID");
        if (string.Equals(NormalizeEnum(characterId), "None", StringComparison.Ordinal))
        {
            return null;
        }

        var spawnerId = RequireString(row["SpawnerID"], $"{context}.SpawnerID");
        var sourceLocation = RequireObject(row["Location"], $"{context}.Location");
        var x = RequireFiniteDouble(sourceLocation["X"], $"{context}.Location.X");
        var y = RequireFiniteDouble(sourceLocation["Y"], $"{context}.Location.Y");
        var level = RequireInt(row["Level"], $"{context}.Level");
        var monster = RequireRow(monsterRows, characterId, MonsterParameterPackage);
        if (!RequireBool(monster["IsPal"], $"{MonsterParameterPackage}[{characterId}].IsPal") ||
            !RequireBool(monster["IsBoss"], $"{MonsterParameterPackage}[{characterId}].IsBoss") ||
            RequireBool(monster["IsTowerBoss"], $"{MonsterParameterPackage}[{characterId}].IsTowerBoss"))
        {
            throw new InvalidOperationException(
                $"{MonsterParameterPackage}[{characterId}] is not a field Alpha (expected IsPal=true, IsBoss=true, IsTowerBoss=false).");
        }

        var name = ResolvePalName(monster, palNameRows, characterId);
        var elements = ReadElements(monster);
        var detail = elements.Count == 0 ? "Field Alpha" : $"Field Alpha · {string.Join(" / ", elements)}";
        var coordinateId = string.Create(CultureInfo.InvariantCulture, $"{x:F2}:{y:F2}");
        return new ShapedAlpha(
            new LandmarkLocation(
                $"alpha:{spawnerId}:{coordinateId}",
                "alpha-pals",
                name,
                detail,
                level,
                x,
                y),
            spawnerId);
    }

    internal static LandmarkLocation ShapeTower(
        TowerPlacement placement,
        JObject monsterRows,
        JObject palNameRows,
        JObject regionRows,
        IReadOnlyDictionary<string, BossParameter> bossParameters)
    {
        var bossType = NormalizeEnum(placement.BossType);
        if (!TowerRegions.TryGetValue(bossType, out var region))
        {
            throw new InvalidOperationException($"Unmapped placed tower BossType {bossType}.");
        }
        if (!bossParameters.TryGetValue(bossType, out var bossParameter))
        {
            throw new InvalidOperationException($"BossInfoMap has no Normal difficulty parameters for placed BossType {bossType}.");
        }
        if (!double.IsFinite(placement.X) || !double.IsFinite(placement.Y))
        {
            throw new InvalidOperationException($"Tower actor {placement.ActorName} has non-finite coordinates.");
        }

        var monster = RequireRow(monsterRows, bossParameter.PalId, MonsterParameterPackage);
        if (!RequireBool(monster["IsTowerBoss"], $"{MonsterParameterPackage}[{bossParameter.PalId}].IsTowerBoss"))
        {
            throw new InvalidOperationException($"{MonsterParameterPackage}[{bossParameter.PalId}] is not marked as a tower boss.");
        }
        var name = ResolvePalName(monster, palNameRows, bossParameter.PalId);
        var regionRow = RequireRow(regionRows, region.TextRow, RegionNamePackage);
        var regionName = ResolveLocalizedText(regionRow, $"{RegionNamePackage}[{region.TextRow}]");
        var elements = ReadElements(monster);
        var detail = elements.Count == 0 ? regionName : $"{regionName} · {string.Join(" / ", elements)}";

        return new LandmarkLocation(
            $"tower:{region.Id}",
            "bosses",
            name,
            detail,
            bossParameter.Level,
            placement.X,
            placement.Y);
    }

    internal static void AssertUniqueLocationIds(IReadOnlyCollection<LandmarkLocation> locations)
    {
        var duplicates = locations
            .GroupBy(item => item.Id, StringComparer.Ordinal)
            .Where(group => group.Count() > 1)
            .Select(group => group.Key)
            .OrderBy(item => item, StringComparer.Ordinal)
            .ToArray();
        if (duplicates.Length != 0)
        {
            throw new InvalidOperationException($"Duplicate generated landmark IDs: {string.Join(", ", duplicates)}.");
        }
    }

    internal static JObject RequireRow(JObject rows, string key, string packagePath)
    {
        var property = rows.Property(key, StringComparison.Ordinal);
        if (property == null)
        {
            throw new InvalidOperationException($"{packagePath} has no row named {key}.");
        }
        return RequireObject(property.Value, $"{packagePath}[{key}]");
    }

    internal static string ResolveLocalizedText(JObject row, string context)
    {
        var textData = RequireObject(row["TextData"], $"{context}.TextData");
        return RequireString(textData["LocalizedString"], $"{context}.TextData.LocalizedString");
    }

    internal static JObject RequireObject(JToken? token, string context) =>
        token as JObject ?? throw new InvalidOperationException($"Expected an object at {context}.");

    internal static JArray RequireArray(JToken? token, string context) =>
        token as JArray ?? throw new InvalidOperationException($"Expected an array at {context}.");

    internal static string RequireString(JToken? token, string context)
    {
        var value = token?.Type == JTokenType.String ? token.Value<string>()?.Trim() : null;
        return string.IsNullOrEmpty(value)
            ? throw new InvalidOperationException($"Expected a non-empty string at {context}.")
            : value;
    }

    internal static int RequireInt(JToken? token, string context)
    {
        if (token is not JValue scalar || token.Type is not (JTokenType.Integer or JTokenType.Float))
        {
            throw new InvalidOperationException($"Expected an integer at {context}.");
        }
        try
        {
            var value = Convert.ToDouble(scalar.Value, CultureInfo.InvariantCulture);
            if (!double.IsFinite(value) || value != Math.Truncate(value) || value < int.MinValue || value > int.MaxValue)
            {
                throw new InvalidOperationException($"Expected an integer at {context}.");
            }
            return checked((int)value);
        }
        catch (Exception exception) when (exception is FormatException or InvalidCastException or OverflowException)
        {
            throw new InvalidOperationException($"Expected an integer at {context}.", exception);
        }
    }

    internal static string NormalizeEnum(string value)
    {
        var trimmed = value.Trim();
        var separator = trimmed.LastIndexOf("::", StringComparison.Ordinal);
        return separator < 0 ? trimmed : trimmed[(separator + 2)..];
    }

    private static string ResolvePalName(JObject monster, JObject palNameRows, string monsterId)
    {
        var nameTextId = RequireString(monster["OverrideNameTextID"], $"{MonsterParameterPackage}[{monsterId}].OverrideNameTextID");
        if (NormalizeEnum(nameTextId) == "None")
        {
            throw new InvalidOperationException($"{MonsterParameterPackage}[{monsterId}] has no OverrideNameTextID.");
        }
        var nameRow = RequireRow(palNameRows, nameTextId, PalNamePackage);
        return ResolveLocalizedText(nameRow, $"{PalNamePackage}[{nameTextId}]");
    }

    private static List<string> ReadElements(JObject monster)
    {
        var elements = new List<string>(2);
        foreach (var property in new[] { "ElementType1", "ElementType2" })
        {
            var element = DisplayElement(NormalizeEnum(RequireString(monster[property], $"monster.{property}")));
            if (element != "None" && !elements.Contains(element, StringComparer.Ordinal))
            {
                elements.Add(element);
            }
        }
        return elements;
    }

    private static bool RequireBool(JToken? token, string context)
    {
        if (token?.Type != JTokenType.Boolean)
        {
            throw new InvalidOperationException($"Expected a boolean at {context}.");
        }
        return token.Value<bool>();
    }

    private static double RequireFiniteDouble(JToken? token, string context)
    {
        if (token is not JValue scalar || token.Type is not (JTokenType.Integer or JTokenType.Float))
        {
            throw new InvalidOperationException($"Expected a finite number at {context}.");
        }
        try
        {
            var value = Convert.ToDouble(scalar.Value, CultureInfo.InvariantCulture);
            return double.IsFinite(value)
                ? value
                : throw new InvalidOperationException($"Expected a finite number at {context}.");
        }
        catch (Exception exception) when (exception is FormatException or InvalidCastException or OverflowException)
        {
            throw new InvalidOperationException($"Expected a finite number at {context}.", exception);
        }
    }

    private static string DisplayElement(string element) => element switch
    {
        "Electricity" => "Electric",
        "Leaf" => "Grass",
        "Earth" => "Ground",
        _ => element
    };

    private sealed record TowerRegion(string Id, string TextRow);
}

internal sealed record ShapedAlpha(LandmarkLocation Location, string SpawnerId);
internal sealed record TowerPlacement(string ActorName, string BossType, double X, double Y);
internal sealed record BossParameter(string PalId, int Level);
