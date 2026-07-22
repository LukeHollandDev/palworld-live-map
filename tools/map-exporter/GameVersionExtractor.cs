using System.Text;
using System.Text.RegularExpressions;
using CUE4Parse.FileProvider;

internal static class GameVersionExtractor
{
    internal const string SourcePath = "Pal/Config/DefaultGame.ini";

    private const string ProjectSettingsSection = "/Script/EngineSettings.GeneralProjectSettings";
    private const string ProjectVersionKey = "ProjectVersion";
    private const int MaximumFileBytes = 1024 * 1024;
    private static readonly Regex CompleteVersionPattern = new(
        @"^[0-9]+\.[0-9]+\.[0-9]+(?:\.[0-9]+)?$",
        RegexOptions.CultureInvariant | RegexOptions.NonBacktracking);

    internal static string ReadMounted(DefaultFileProvider provider)
    {
        if (!provider.Files.TryGetValue(SourcePath, out var file))
        {
            throw new InvalidOperationException(
                $"Mounted Palworld data does not contain {SourcePath}; cannot determine the catalogue game version.");
        }
        if (file.Size <= 0 || file.Size > MaximumFileBytes)
        {
            throw new InvalidOperationException(
                $"{SourcePath} has an invalid size ({file.Size} bytes); expected 1-{MaximumFileBytes} bytes.");
        }

        var bytes = file.Read();
        if (bytes.Length == 0 || bytes.Length > MaximumFileBytes)
        {
            throw new InvalidOperationException(
                $"{SourcePath} decoded to an invalid size ({bytes.Length} bytes); expected 1-{MaximumFileBytes} bytes.");
        }

        string contents;
        try
        {
            contents = new UTF8Encoding(encoderShouldEmitUTF8Identifier: false, throwOnInvalidBytes: true).GetString(bytes);
        }
        catch (DecoderFallbackException exception)
        {
            throw new InvalidOperationException($"{SourcePath} is not valid UTF-8.", exception);
        }
        return Parse(contents);
    }

    internal static string Parse(string contents)
    {
        if (contents.Length == 0 || Encoding.UTF8.GetByteCount(contents) > MaximumFileBytes)
        {
            throw new InvalidOperationException(
                $"{SourcePath} text must contain 1-{MaximumFileBytes} UTF-8 bytes.");
        }
        if (contents[0] == '\uFEFF')
        {
            contents = contents[1..];
        }

        var sectionCount = 0;
        var keyCount = 0;
        var inProjectSettings = false;
        string? projectVersion = null;
        using var reader = new StringReader(contents);
        while (reader.ReadLine() is { } line)
        {
            var trimmed = line.Trim();
            if (trimmed.Length == 0 || trimmed.StartsWith(';') || trimmed.StartsWith('#'))
            {
                continue;
            }
            if (trimmed.StartsWith('['))
            {
                if (!trimmed.EndsWith(']'))
                {
                    throw new InvalidOperationException(
                        $"Malformed INI section header in {SourcePath}: {trimmed}");
                }
                var section = trimmed[1..^1].Trim();
                inProjectSettings = string.Equals(section, ProjectSettingsSection, StringComparison.OrdinalIgnoreCase);
                if (inProjectSettings)
                {
                    sectionCount++;
                }
                continue;
            }
            if (!inProjectSettings)
            {
                continue;
            }

            var equals = trimmed.IndexOf('=');
            if (equals < 0 || !string.Equals(
                    trimmed[..equals].Trim(), ProjectVersionKey, StringComparison.OrdinalIgnoreCase))
            {
                continue;
            }
            keyCount++;
            projectVersion = trimmed[(equals + 1)..].Trim();
        }

        if (sectionCount != 1)
        {
            throw new InvalidOperationException(
                $"Expected exactly one [{ProjectSettingsSection}] section in {SourcePath}; found {sectionCount}.");
        }
        if (keyCount != 1)
        {
            throw new InvalidOperationException(
                $"Expected exactly one {ProjectVersionKey} key in [{ProjectSettingsSection}]; found {keyCount}.");
        }
        if (!IsCompleteVersion(projectVersion))
        {
            throw new InvalidOperationException(
                $"{ProjectVersionKey} in {SourcePath} must be a complete three- or four-component numeric release; got {projectVersion ?? "<missing>"}.");
        }
        return projectVersion!;
    }

    internal static bool IsCompleteVersion(string? value) =>
        value is not null && CompleteVersionPattern.IsMatch(value);
}
