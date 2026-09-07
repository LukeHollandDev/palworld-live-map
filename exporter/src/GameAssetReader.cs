using CUE4Parse.FileProvider;
using CUE4Parse.UE4.Assets.Exports;
using CUE4Parse.UE4.Objects.Core.Math;
using Newtonsoft.Json;
using Newtonsoft.Json.Linq;

// Small, strict adapters around CUE4Parse. Keeping package decoding here
// avoids every catalogue extractor inventing its own reference and placement
// rules.
internal static class GameAssetReader
{
    internal static JObject LoadRows(DefaultFileProvider provider, string packagePath)
    {
        var table = LoadDataTable(provider, packagePath);
        return ResolveRows(provider, table, packagePath, new HashSet<string>(StringComparer.Ordinal) { packagePath });
    }

    private static JObject LoadDataTable(DefaultFileProvider provider, string packagePath)
    {
        var tables = LoadExports(provider, packagePath)
            .Where(item => (item.Class?.Name.ToString() ?? string.Empty).Contains("DataTable", StringComparison.Ordinal))
            .ToArray();
        if (tables.Length != 1)
        {
            throw new InvalidOperationException($"Expected one DataTable export in {packagePath}, found {tables.Length}.");
        }
        return SerializeObject(tables[0]);
    }

    // Palworld 1.0.4 split several data tables into CompositeDataTable shells whose own
    // Rows are empty and whose effective rows live in ParentTables. Resolve the
    // inheritance chain so callers see the full row set: parent rows load in order and
    // the composite's own rows override them, matching engine lookup. Fail closed on
    // cycles and malformed references.
    internal static JObject ResolveRows(
        DefaultFileProvider provider,
        JObject table,
        string packagePath,
        ISet<string> visited)
    {
        var ownRows = LandmarkShaper.RequireObject(table["Rows"], $"{packagePath}.Rows");
        var parentReferences = table["Properties"]?["ParentTables"] as JArray;
        if (parentReferences is null || parentReferences.Count == 0)
        {
            return ownRows;
        }

        var parentRowSets = new List<JObject>(parentReferences.Count);
        foreach (var reference in parentReferences)
        {
            var parentObjectPath = reference?["ObjectPath"]?.ToString();
            var separator = parentObjectPath?.LastIndexOf('.');
            if (string.IsNullOrEmpty(parentObjectPath) || separator is null || separator.Value < 1)
            {
                throw new InvalidOperationException(
                    $"{packagePath} has a malformed parent table reference: {reference}");
            }
            var parentPackagePath = parentObjectPath[..separator.Value];
            if (!visited.Add(parentPackagePath))
            {
                throw new InvalidOperationException(
                    $"CompositeDataTable inheritance cycle at {parentPackagePath} referenced from {packagePath}.");
            }
            var parentTable = LoadDataTable(provider, parentPackagePath);
            parentRowSets.Add(ResolveRows(provider, parentTable, parentPackagePath, visited));
        }
        return MergeTableRows(parentRowSets, ownRows);
    }

    // Pure parent/own row merge kept separate from CUE4Parse so override order and
    // row cloning stay testable against fixtures.
    internal static JObject MergeTableRows(IReadOnlyList<JObject> parentRowSets, JObject ownRows)
    {
        var merged = new JObject();
        foreach (var rows in parentRowSets)
        {
            foreach (var property in rows.Properties())
            {
                merged[property.Name] = property.Value.DeepClone();
            }
        }
        foreach (var property in ownRows.Properties())
        {
            merged[property.Name] = property.Value.DeepClone();
        }
        return merged;
    }

    internal static UObject[] LoadExports(DefaultFileProvider provider, string packagePath) =>
        provider.LoadPackage(packagePath).GetExports().ToArray();

    internal static PlacedActor ReadPlacement(UObject actor, string sourcePackage)
    {
        var className = actor.Class?.Name.ToString();
        if (string.IsNullOrWhiteSpace(className))
        {
            throw new InvalidOperationException($"Placed export {actor.Name} in {sourcePackage} has no class name.");
        }

        var context = $"{className} actor {actor.Name} in {sourcePackage}";
        var actorJson = SerializeObject(actor);
        var properties = LandmarkShaper.RequireObject(actorJson["Properties"], $"{context}.Properties");
        if (!actor.TryGetValue<UObject>(out var rootComponent, "RootComponent"))
        {
            throw new InvalidOperationException($"{context} has no loadable RootComponent.");
        }
        if (!rootComponent.TryGetValue<FVector>(out var position, "RelativeLocation"))
        {
            throw new InvalidOperationException($"{context} RootComponent has no RelativeLocation.");
        }
        if (!double.IsFinite(position.X) || !double.IsFinite(position.Y) || !double.IsFinite(position.Z))
        {
            throw new InvalidOperationException($"{context} has non-finite coordinates.");
        }

        return new PlacedActor(
            actor.Name.ToString(),
            className,
            sourcePackage,
            properties,
            position.X,
            position.Y,
            position.Z);
    }

    internal static JObject SerializeObject(UObject value) =>
        JObject.Parse(JsonConvert.SerializeObject(value));

    internal static string ObjectPath(string packagePath)
    {
        var name = packagePath[(packagePath.LastIndexOf('/') + 1)..];
        return $"{packagePath}.{name}";
    }
}

internal sealed record PlacedActor(
    string ActorName,
    string ClassName,
    string SourcePackage,
    JObject Properties,
    double X,
    double Y,
    double Z);
