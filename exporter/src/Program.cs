using System.Reflection;
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
const string GeneratorName = "palworld-asset-exporter";
const string GeneratorVersion = "3";

var options = ParseOptions(args);
using var mapOutput = StagedOutputDirectory.Create(options.OutputDirectory);
using var landmarkOutput = StagedOutputDirectory.Create(options.LandmarkOutputDirectory);

var pakFiles = FindPakFiles(options.PakDirectory);
if (pakFiles.Length == 0)
{
    throw new InvalidOperationException($"No .pak files found in {options.PakDirectory}");
}

Console.WriteLine($"Hashing {pakFiles.Length} source PAK file(s) before extraction...");
var pakManifest = SourceSnapshots.CaptureFiles(pakFiles);
var mappingsManifest = SourceSnapshots.CaptureFile(options.MappingsFile);
Console.WriteLine("Captured initial PAK provenance; mounting verified sources...");

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
var gameVersion = GameVersionExtractor.ReadMounted(provider);
if (options.ExpectedGameVersion is not null &&
    !string.Equals(options.ExpectedGameVersion, gameVersion, StringComparison.Ordinal))
{
    throw new InvalidOperationException(
        $"Installed Palworld ProjectVersion is {gameVersion}, but --game-version expected {options.ExpectedGameVersion}.");
}
Console.WriteLine($"Detected Palworld ProjectVersion {gameVersion} from {GameVersionExtractor.SourcePath}.");

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

    var destination = Path.Combine(mapOutput.PayloadDirectory, layer.FileName);
    image.Save(destination, new JpegEncoder { Quality = 90 });
    var outputHash = SourceSnapshots.HashFile(destination);
    Console.WriteLine($"Staged {layer.FileName} ({sourceWidth}x{sourceHeight} -> {OutputSize}x{OutputSize})");
    exported.Add(new LayerManifest(
        layer.Id, layer.Name, layer.FileName, objectPath, layer.Bounds,
        sourceWidth, sourceHeight, OutputSize, OutputSize, outputHash));
}

var cue4ParseVersion = GetAssemblyMetadata("CUE4ParseVersion");
var manifest = new MapManifest(
    1,
    gameVersion,
    $"{GeneratorName}/{GeneratorVersion}",
    $"CUE4Parse/{cue4ParseVersion}",
    mappingsManifest,
    pakManifest,
    exported);

var manifestPath = Path.Combine(mapOutput.PayloadDirectory, "manifest.json");
File.WriteAllText(manifestPath, JsonSerializer.Serialize(manifest, new JsonSerializerOptions
{
    WriteIndented = true,
    PropertyNamingPolicy = JsonNamingPolicy.CamelCase
}) + Environment.NewLine);
Console.WriteLine("Staged map manifest.json");

LandmarkExporter.Export(
    provider,
    landmarkOutput.PayloadDirectory,
    gameVersion,
    $"{GeneratorName}/{GeneratorVersion}",
    $"CUE4Parse/{cue4ParseVersion}",
    mappingsManifest,
    pakManifest);

Console.WriteLine("Re-hashing source PAKs to verify provenance coherence...");
var finalPakManifest = SourceSnapshots.CaptureFiles(FindPakFiles(options.PakDirectory));
SourceSnapshots.EnsureUnchanged("Palworld source PAKs", pakManifest, finalPakManifest);
Console.WriteLine("Re-hashing the mappings file to verify provenance coherence...");
var finalMappingsManifest = SourceSnapshots.CaptureFile(options.MappingsFile);
SourceSnapshots.EnsureUnchanged("Palworld mappings file", [mappingsManifest], [finalMappingsManifest]);
Console.WriteLine("Verified that every source PAK and the mappings file remained unchanged throughout the export.");

OutputPromotion.Promote(mapOutput, landmarkOutput);
Console.WriteLine($"Promoted map output to {mapOutput.DestinationDirectory}");
Console.WriteLine($"Promoted landmark output to {landmarkOutput.DestinationDirectory}");

static ExportOptions ParseOptions(string[] arguments)
{
    string? pakDirectory = null;
    string? mappingsFile = null;
    string? outputDirectory = null;
    string? landmarkOutputDirectory = null;
    string? gameVersion = null;

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
            case "--landmark-output": landmarkOutputDirectory = value; break;
            case "--game-version": gameVersion = value; break;
            default: throw new ArgumentException($"Unknown option: {key}");
        }
    }

    if (string.IsNullOrWhiteSpace(pakDirectory) || string.IsNullOrWhiteSpace(mappingsFile) ||
        string.IsNullOrWhiteSpace(outputDirectory) || string.IsNullOrWhiteSpace(landmarkOutputDirectory))
    {
        throw new ArgumentException(
            "Usage: PalworldAssetExporter --pak-directory PATH --mappings FILE --output PATH " +
            "--landmark-output PATH [--game-version EXPECTED_VERSION]");
    }
    gameVersion = gameVersion?.Trim();
    if (gameVersion is not null && !GameVersionExtractor.IsCompleteVersion(gameVersion))
    {
        throw new ArgumentException(
            "--game-version is an optional assertion and must be a complete three- or four-component numeric Palworld release when supplied.");
    }
    if (!Directory.Exists(pakDirectory)) throw new DirectoryNotFoundException(pakDirectory);
    if (!File.Exists(mappingsFile)) throw new FileNotFoundException("Mappings file not found", mappingsFile);
    return new ExportOptions(pakDirectory, mappingsFile, outputDirectory, landmarkOutputDirectory, gameVersion);
}

static string[] FindPakFiles(string pakDirectory) => Directory
    .GetFiles(pakDirectory, "*.pak", SearchOption.TopDirectoryOnly)
    .OrderBy(Path.GetFileName, StringComparer.Ordinal)
    .ToArray();

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

record ExportOptions(
    string PakDirectory,
    string MappingsFile,
    string OutputDirectory,
    string LandmarkOutputDirectory,
    string? ExpectedGameVersion);
record LayerSource(string Id, string Name, string FileName, string ObjectPath, double[] Bounds);
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
