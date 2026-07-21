using System.Reflection;
using System.Security.Cryptography;
using System.Text.Json;
using CUE4Parse.FileProvider;
using CUE4Parse.MappingsProvider.Usmap;
using CUE4Parse.UE4.Assets.Exports.Texture;
using CUE4Parse.UE4.Versions;
using CUE4Parse_Conversion.Textures;
using SixLabors.ImageSharp;
using SixLabors.ImageSharp.Formats.Jpeg;
using SixLabors.ImageSharp.Processing;

const int OutputSize = 8192;
const string GeneratorVersion = "1";

var options = ParseOptions(args);
Directory.CreateDirectory(options.OutputDirectory);

var pakFiles = Directory.GetFiles(options.PakDirectory, "*.pak", SearchOption.TopDirectoryOnly);
if (pakFiles.Length == 0)
{
    throw new InvalidOperationException($"No .pak files found in {options.PakDirectory}");
}

Console.WriteLine($"Indexing {pakFiles.Length} Palworld PAK file(s)...");
var versions = new VersionContainer(EGame.GAME_UE5_1, ETexturePlatform.DesktopMobile);
var provider = new DefaultFileProvider(options.PakDirectory, SearchOption.TopDirectoryOnly, versions, StringComparer.OrdinalIgnoreCase)
{
    MappingsContainer = new FileUsmapTypeMappingsProvider(options.MappingsFile)
};
provider.Initialize();
provider.Mount();
provider.PostMount();
Console.WriteLine($"Mounted {provider.Files.Count} game files.");

var layers = new[]
{
    new LayerSource(
        "palpagos", "Palpagos", "palpagos.jpg",
        "Pal/Content/Pal/Texture/UI/Map/T_WorldMap.T_WorldMap",
        new[] { 349400d, 724400d, -1099400d, -724400d }),
    new LayerSource(
        "world-tree", "World Tree", "world-tree.jpg",
        "Pal/Content/Pal/Texture/UI/Map/T_TreeMap.T_TreeMap",
        new[] { 689148.5d, -476400d, 347351.5d, -818197d })
};

var exported = new List<LayerManifest>();
foreach (var layer in layers)
{
    var objectPath = ResolveObjectPath(provider, layer.ObjectPath);
    Console.WriteLine($"Decoding {objectPath}...");
    var texture = provider.LoadPackageObject<UTexture2D>(objectPath);
    var decoded = texture.Decode(ETexturePlatform.DesktopMobile)
        ?? throw new InvalidOperationException($"CUE4Parse could not decode {objectPath}");
    var png = decoded.Encode(ETextureFormat.Png, false, out _);
    using var image = Image.Load(png);
    var sourceWidth = image.Width;
    var sourceHeight = image.Height;
    if (image.Width != OutputSize || image.Height != OutputSize)
    {
        image.Mutate(operation => operation.Resize(new ResizeOptions
        {
            Size = new Size(OutputSize, OutputSize),
            Mode = ResizeMode.Stretch,
            Sampler = KnownResamplers.Lanczos3
        }));
    }
    image.Metadata.ExifProfile = null;
    image.Metadata.IccProfile = null;
    image.Metadata.XmpProfile = null;

    var destination = Path.Combine(options.OutputDirectory, layer.FileName);
    image.Save(destination, new JpegEncoder { Quality = 90 });
    var outputHash = HashFile(destination);
    Console.WriteLine($"Wrote {destination} ({sourceWidth}x{sourceHeight} -> {OutputSize}x{OutputSize})");
    exported.Add(new LayerManifest(
        layer.Id, layer.Name, layer.FileName, objectPath, layer.Bounds,
        sourceWidth, sourceHeight, OutputSize, OutputSize, outputHash));
}

var pakManifest = pakFiles
    .OrderBy(Path.GetFileName, StringComparer.Ordinal)
    .Select(path => new SourceFile(Path.GetFileName(path), new FileInfo(path).Length, HashFile(path)))
    .ToArray();
var cue4ParseVersion = GetAssemblyMetadata("CUE4ParseVersion");
var manifest = new MapManifest(
    1,
    options.GameVersion,
    $"palworld-map-exporter/{GeneratorVersion}",
    $"CUE4Parse/{cue4ParseVersion}",
    new SourceFile(Path.GetFileName(options.MappingsFile), new FileInfo(options.MappingsFile).Length, HashFile(options.MappingsFile)),
    pakManifest,
    exported);

var manifestPath = Path.Combine(options.OutputDirectory, "manifest.json");
File.WriteAllText(manifestPath, JsonSerializer.Serialize(manifest, new JsonSerializerOptions
{
    WriteIndented = true,
    PropertyNamingPolicy = JsonNamingPolicy.CamelCase
}) + Environment.NewLine);
Console.WriteLine($"Wrote {manifestPath}");

static ExportOptions ParseOptions(string[] arguments)
{
    string? pakDirectory = null;
    string? mappingsFile = null;
    string? outputDirectory = null;
    var gameVersion = "unknown";

    for (var index = 0; index < arguments.Length; index++)
    {
        var key = arguments[index];
        if (index + 1 >= arguments.Length)
        {
            throw new ArgumentException($"Missing value for {key}");
        }
        var value = arguments[++index];
        switch (key)
        {
            case "--pak-directory": pakDirectory = value; break;
            case "--mappings": mappingsFile = value; break;
            case "--output": outputDirectory = value; break;
            case "--game-version": gameVersion = value; break;
            default: throw new ArgumentException($"Unknown option: {key}");
        }
    }

    if (string.IsNullOrWhiteSpace(pakDirectory) || string.IsNullOrWhiteSpace(mappingsFile) || string.IsNullOrWhiteSpace(outputDirectory))
    {
        throw new ArgumentException("Usage: MapExporter --pak-directory PATH --mappings FILE --output PATH [--game-version VERSION]");
    }
    if (!Directory.Exists(pakDirectory)) throw new DirectoryNotFoundException(pakDirectory);
    if (!File.Exists(mappingsFile)) throw new FileNotFoundException("Mappings file not found", mappingsFile);
    return new ExportOptions(pakDirectory, mappingsFile, outputDirectory, gameVersion);
}

static string HashFile(string path)
{
    using var stream = File.OpenRead(path);
    return Convert.ToHexString(SHA256.HashData(stream)).ToLowerInvariant();
}

static string GetAssemblyMetadata(string key)
{
    var value = Assembly.GetExecutingAssembly()
        .GetCustomAttributes<AssemblyMetadataAttribute>()
        .SingleOrDefault(attribute => attribute.Key == key)
        ?.Value;
    return string.IsNullOrWhiteSpace(value)
        ? throw new InvalidOperationException($"Missing assembly metadata: {key}")
        : value;
}

static string ResolveObjectPath(DefaultFileProvider provider, string documentedObjectPath)
{
    var separator = documentedObjectPath.LastIndexOf('.');
    var objectName = documentedObjectPath[(separator + 1)..];
    var suffix = $"/{objectName}.uasset";
    var matches = provider.Files.Keys
        .Where(path => path.EndsWith(suffix, StringComparison.OrdinalIgnoreCase))
        .ToArray();
    if (matches.Length != 1)
    {
        var nearby = provider.Files.Keys
            .Where(path => path.Contains(objectName, StringComparison.OrdinalIgnoreCase) || path.Contains("WorldMap", StringComparison.OrdinalIgnoreCase))
            .Take(10);
        throw new InvalidOperationException(
            $"Expected exactly one mounted {objectName}.uasset, found {matches.Length}. " +
            $"Nearby paths: {string.Join(", ", nearby)}. " +
            "The installed Palworld version or mappings may not be supported.");
    }
    return matches[0][..^".uasset".Length] + $".{objectName}";
}

record ExportOptions(string PakDirectory, string MappingsFile, string OutputDirectory, string GameVersion);
record LayerSource(string Id, string Name, string FileName, string ObjectPath, double[] Bounds);
record SourceFile(string File, long Bytes, string Sha256);
record LayerManifest(
    string Id,
    string Name,
    string File,
    string SourceObject,
    double[] Bounds,
    int SourceWidth,
    int SourceHeight,
    int Width,
    int Height,
    string Sha256);
record MapManifest(
    int SchemaVersion,
    string GameVersion,
    string Generator,
    string Decoder,
    SourceFile Mappings,
    SourceFile[] Paks,
    List<LayerManifest> Layers);
