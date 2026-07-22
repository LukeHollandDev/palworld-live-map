using System.Security.Cryptography;

internal static class SourceSnapshots
{
    internal static SourceFile[] CaptureFiles(IEnumerable<string> paths) => paths
        .Select(CaptureFile)
        .OrderBy(source => source.File, StringComparer.Ordinal)
        .ToArray();

    internal static SourceFile CaptureFile(string path)
    {
        var before = new FileInfo(path);
        before.Refresh();
        if (!before.Exists)
        {
            throw new FileNotFoundException("Source file disappeared before it could be hashed", path);
        }

        var initialLength = before.Length;
        var sha256 = HashFile(path);
        var after = new FileInfo(path);
        after.Refresh();
        if (!after.Exists || after.Length != initialLength)
        {
            throw new InvalidOperationException($"Source file changed while it was being hashed: {path}");
        }

        return new SourceFile(Path.GetFileName(path), initialLength, sha256);
    }

    internal static void EnsureUnchanged(
        string description,
        IReadOnlyList<SourceFile> initial,
        IReadOnlyList<SourceFile> final)
    {
        if (initial.SequenceEqual(final))
        {
            return;
        }

        static string Describe(IEnumerable<SourceFile> files) => string.Join(
            ", ",
            files.Select(file => $"{file.File} ({file.Bytes} bytes, sha256:{file.Sha256})"));

        throw new InvalidOperationException(
            $"{description} changed during export. Generated output was not promoted; rerun against stable inputs. " +
            $"Initial: [{Describe(initial)}]. Final: [{Describe(final)}].");
    }

    internal static string HashFile(string path)
    {
        using var stream = File.OpenRead(path);
        return Convert.ToHexString(SHA256.HashData(stream)).ToLowerInvariant();
    }
}

internal sealed class StagedOutputDirectory : IDisposable
{
    private const string StagingPrefix = ".palworld-map-exporter-stage-";
    private bool disposed;
    private bool preserveForRecovery;

    private StagedOutputDirectory(string destinationDirectory, string stagingRoot)
    {
        DestinationDirectory = destinationDirectory;
        StagingRoot = stagingRoot;
        PayloadDirectory = Path.Combine(stagingRoot, "payload");
        BackupDirectory = Path.Combine(stagingRoot, "backup");
        Directory.CreateDirectory(PayloadDirectory);
    }

    internal string DestinationDirectory { get; }
    internal string PayloadDirectory { get; }
    internal string StagingRoot { get; }
    internal string BackupDirectory { get; }

    internal void PreserveForRecovery() => preserveForRecovery = true;

    internal static StagedOutputDirectory Create(string destinationDirectory)
    {
        var destination = Path.GetFullPath(destinationDirectory);
        Directory.CreateDirectory(destination);

        string stagingRoot;
        do
        {
            stagingRoot = Path.Combine(destination, $"{StagingPrefix}{Guid.NewGuid():N}");
        }
        while (Directory.Exists(stagingRoot) || File.Exists(stagingRoot));

        Directory.CreateDirectory(stagingRoot);
        return new StagedOutputDirectory(destination, stagingRoot);
    }

    public void Dispose()
    {
        if (disposed)
        {
            return;
        }

        disposed = true;
        if (!preserveForRecovery && Directory.Exists(StagingRoot))
        {
            Directory.Delete(StagingRoot, recursive: true);
        }
    }
}

internal static class OutputPromotion
{
    internal static void Promote(params StagedOutputDirectory[] outputDirectories)
    {
        if (outputDirectories.Length == 0)
        {
            throw new ArgumentException("At least one staged output directory is required.", nameof(outputDirectories));
        }

        var entries = outputDirectories
            .SelectMany(CreateEntries)
            .OrderBy(entry => entry.DestinationPath, StringComparer.Ordinal)
            .ToArray();
        if (entries.Length == 0)
        {
            throw new InvalidOperationException("No staged output files were found to promote.");
        }

        var duplicateTargets = entries
            .GroupBy(entry => entry.DestinationPath, StringComparer.OrdinalIgnoreCase)
            .Where(group => group.Count() > 1)
            .Select(group => group.Key)
            .OrderBy(path => path, StringComparer.Ordinal)
            .ToArray();
        if (duplicateTargets.Length != 0)
        {
            throw new InvalidOperationException(
                $"Staged outputs resolve to duplicate destination files: {string.Join(", ", duplicateTargets)}");
        }

        foreach (var entry in entries)
        {
            if (Directory.Exists(entry.DestinationPath))
            {
                throw new InvalidOperationException($"Output file destination is an existing directory: {entry.DestinationPath}");
            }

            Directory.CreateDirectory(Path.GetDirectoryName(entry.DestinationPath)!);
        }

        try
        {
            // Preserve every old output before moving any new output into place. Backups
            // live inside the destination filesystem, so both backup and promotion moves
            // remain same-filesystem renames even when map and landmark outputs differ.
            foreach (var entry in entries.Where(entry => File.Exists(entry.DestinationPath)))
            {
                Directory.CreateDirectory(Path.GetDirectoryName(entry.BackupPath)!);
                File.Move(entry.DestinationPath, entry.BackupPath);
                entry.BackedUp = true;
            }

            foreach (var entry in entries)
            {
                File.Move(entry.StagedPath, entry.DestinationPath);
                entry.Promoted = true;
            }
        }
        catch (Exception promotionError)
        {
            var rollbackErrors = RollBack(entries);
            if (rollbackErrors.Count != 0)
            {
                foreach (var output in outputDirectories)
                {
                    output.PreserveForRecovery();
                }
                var recoveryPaths = string.Join(", ", outputDirectories.Select(output => output.StagingRoot));
                throw new AggregateException(
                    "Output promotion failed and one or more previous output files could not be restored. " +
                    $"Staged files and backups were preserved for manual recovery at: {recoveryPaths}",
                    new[] { promotionError }.Concat(rollbackErrors));
            }

            throw new InvalidOperationException(
                "Output promotion failed; all previous output files were restored.",
                promotionError);
        }
    }

    private static IEnumerable<PromotionEntry> CreateEntries(StagedOutputDirectory output)
    {
        foreach (var stagedPath in Directory
                     .GetFiles(output.PayloadDirectory, "*", SearchOption.AllDirectories)
                     .OrderBy(path => path, StringComparer.Ordinal))
        {
            var relativePath = Path.GetRelativePath(output.PayloadDirectory, stagedPath);
            yield return new PromotionEntry(
                stagedPath,
                Path.GetFullPath(Path.Combine(output.DestinationDirectory, relativePath)),
                Path.GetFullPath(Path.Combine(output.BackupDirectory, relativePath)));
        }
    }

    private static List<Exception> RollBack(IEnumerable<PromotionEntry> entries)
    {
        var errors = new List<Exception>();
        foreach (var entry in entries.Reverse())
        {
            try
            {
                if (entry.Promoted && File.Exists(entry.DestinationPath))
                {
                    Directory.CreateDirectory(Path.GetDirectoryName(entry.StagedPath)!);
                    File.Move(entry.DestinationPath, entry.StagedPath);
                }

                if (entry.BackedUp && File.Exists(entry.BackupPath))
                {
                    File.Move(entry.BackupPath, entry.DestinationPath);
                }
            }
            catch (Exception exception)
            {
                errors.Add(exception);
            }
        }
        return errors;
    }

    private sealed class PromotionEntry(string stagedPath, string destinationPath, string backupPath)
    {
        internal string StagedPath { get; } = stagedPath;
        internal string DestinationPath { get; } = destinationPath;
        internal string BackupPath { get; } = backupPath;
        internal bool BackedUp { get; set; }
        internal bool Promoted { get; set; }
    }
}

internal sealed record SourceFile(string File, long Bytes, string Sha256);
