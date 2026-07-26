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
        var tables = LoadExports(provider, packagePath)
            .Where(item => (item.Class?.Name.ToString() ?? string.Empty).Contains("DataTable", StringComparison.Ordinal))
            .ToArray();
        if (tables.Length != 1)
        {
            throw new InvalidOperationException($"Expected one DataTable export in {packagePath}, found {tables.Length}.");
        }

        var table = SerializeObject(tables[0]);
        return LandmarkShaper.RequireObject(table["Rows"], $"{packagePath}.Rows");
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
