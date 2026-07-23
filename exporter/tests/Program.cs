using System.Globalization;
using Newtonsoft.Json.Linq;

internal static class TestProgram
{
    private static readonly (string Name, Action Run)[] Tests =
    [
        ("field Alpha joins and output", FieldAlphaJoinsAndOutput),
        ("field Alpha element aliases", FieldAlphaElementAliases),
        ("empty Alpha row is omitted", EmptyAlphaRowIsOmitted),
        ("field Alpha flags fail closed", FieldAlphaFlagsFailClosed),
        ("field Alpha missing and invalid data fail closed", FieldAlphaInvalidDataFailsClosed),
        ("tower joins and output", TowerJoinsAndOutput),
        ("all tower region mappings", AllTowerRegionMappings),
        ("tower missing joins fail closed", TowerMissingJoinsFailClosed),
        ("tower invalid data fails closed", TowerInvalidDataFailsClosed),
        ("duplicate generated IDs fail closed", DuplicateIdsFailClosed),
        ("game version is extracted from DefaultGame.ini", GameVersionIsExtracted),
        ("game version accepts UTF-8 BOM and CRLF", GameVersionAcceptsBomAndCrlf),
        ("game version requires one project settings section", GameVersionRequiresOneSection),
        ("game version rejects malformed section headers", GameVersionRejectsMalformedSection),
        ("game version requires one ProjectVersion key", GameVersionRequiresOneKey),
        ("game version rejects incomplete and moving labels", GameVersionRejectsInvalidLabels),
        ("source snapshots detect changes", SourceSnapshotsDetectChanges),
        ("staged outputs promote across directories", StagedOutputsPromoteAcrossDirectories),
        ("source failure preserves prior outputs", SourceFailurePreservesPriorOutputs),
        ("duplicate promotion targets preserve prior outputs", DuplicatePromotionTargetsPreservePriorOutputs)
    ];

    public static int Main()
    {
        var failures = new List<string>();
        foreach (var (name, run) in Tests)
        {
            try
            {
                run();
                Console.WriteLine($"PASS {name}");
            }
            catch (Exception exception)
            {
                failures.Add($"{name}: {exception.Message}");
                Console.Error.WriteLine($"FAIL {name}\n{exception}");
            }
        }

        Console.WriteLine($"Landmark fixture tests: {Tests.Length - failures.Count} passed, {failures.Count} failed.");
        return failures.Count == 0 ? 0 : 1;
    }

    private static void FieldAlphaJoinsAndOutput()
    {
        var previousCulture = CultureInfo.CurrentCulture;
        try
        {
            // IDs must remain stable on hosts whose decimal separator is not a dot.
            CultureInfo.CurrentCulture = CultureInfo.GetCultureInfo("fr-FR");
            var alpha = ShapeAlpha(LoadObjectFixture("alpha.json"))
                ?? throw new InvalidOperationException("Expected a shaped Alpha.");

            Equal("BossSpawner_Penguin_Emperor", alpha.SpawnerId, "SpawnerID join");
            Equal("alpha:BossSpawner_Penguin_Emperor:-285331.30:210162.69", alpha.Location.Id, "ID");
            Equal("alpha-pals", alpha.Location.Kind, "kind");
            Equal("Penking", alpha.Location.Name, "localized name");
            Equal("Field Alpha · Water / Ice", alpha.Location.Detail, "elements");
            Equal(15, alpha.Location.Level, "level");
            Close(-285331.3, alpha.Location.X, "X");
            Close(210162.69, alpha.Location.Y, "Y");
        }
        finally
        {
            CultureInfo.CurrentCulture = previousCulture;
        }
    }

    private static void FieldAlphaElementAliases()
    {
        var fixture = LoadObjectFixture("alpha.json");
        var monster = AlphaMonster(fixture);
        monster["ElementType1"] = "EPalElementType::Leaf";
        monster["ElementType2"] = "EPalElementType::Earth";

        var alpha = ShapeAlpha(fixture) ?? throw new InvalidOperationException("Expected a shaped Alpha.");
        Equal("Field Alpha · Grass / Ground", alpha.Location.Detail, "element aliases");

        monster["ElementType2"] = "EPalElementType::Leaf";
        alpha = ShapeAlpha(fixture) ?? throw new InvalidOperationException("Expected a shaped Alpha.");
        Equal("Field Alpha · Grass", alpha.Location.Detail, "duplicate elements");
    }

    private static void EmptyAlphaRowIsOmitted()
    {
        var fixture = LoadObjectFixture("alpha.json");
        AlphaRow(fixture)["CharacterID"] = "EPalID::None";
        Equal(null, ShapeAlpha(fixture), "None CharacterID");
    }

    private static void FieldAlphaFlagsFailClosed()
    {
        foreach (var (property, invalidValue) in new[]
        {
            ("IsPal", false),
            ("IsBoss", false),
            ("IsTowerBoss", true)
        })
        {
            var fixture = LoadObjectFixture("alpha.json");
            AlphaMonster(fixture)[property] = invalidValue;
            Throws(() => ShapeAlpha(fixture), "not a field Alpha");
        }

        var missingFlag = LoadObjectFixture("alpha.json");
        AlphaMonster(missingFlag).Remove("IsBoss");
        Throws(() => ShapeAlpha(missingFlag), ".IsBoss");
    }

    private static void FieldAlphaInvalidDataFailsClosed()
    {
        var missingMonster = LoadObjectFixture("alpha.json");
        ObjectAt(missingMonster, "monsterRows").Remove("Penguin_Emperor");
        Throws(() => ShapeAlpha(missingMonster), "has no row named Penguin_Emperor");

        var missingName = LoadObjectFixture("alpha.json");
        ObjectAt(missingName, "palNameRows").Remove("PAL_NAME_Penguin_Emperor");
        Throws(() => ShapeAlpha(missingName), "has no row named PAL_NAME_Penguin_Emperor");

        var noNameKey = LoadObjectFixture("alpha.json");
        AlphaMonster(noNameKey)["OverrideNameTextID"] = "EPalTextId::None";
        Throws(() => ShapeAlpha(noNameKey), "has no OverrideNameTextID");

        var nonFiniteCoordinate = LoadObjectFixture("alpha.json");
        ObjectAt(AlphaRow(nonFiniteCoordinate), "Location")["X"] = double.NaN;
        Throws(() => ShapeAlpha(nonFiniteCoordinate), "finite number");

        var fractionalLevel = LoadObjectFixture("alpha.json");
        AlphaRow(fractionalLevel)["Level"] = 15.5;
        Throws(() => ShapeAlpha(fractionalLevel), "integer");

        var missingElement = LoadObjectFixture("alpha.json");
        AlphaMonster(missingElement).Remove("ElementType2");
        Throws(() => ShapeAlpha(missingElement), "monster.ElementType2");
    }

    private static void TowerJoinsAndOutput()
    {
        var tower = ReadTowerFixture(LoadObjectFixture("tower.json"));
        var location = ShapeTower(tower);

        Equal("tower:REGION_Grass_Boss", location.Id, "ID");
        Equal("bosses", location.Kind, "kind");
        Equal("Zoe & Grizzbolt", location.Name, "localized name");
        Equal("Rayne Syndicate Tower · Electric", location.Detail, "region and element");
        Equal(10, location.Level, "level");
        Close(-321596.25, location.X, "X");
        Close(209085.0, location.Y, "Y");
    }

    private static void AllTowerRegionMappings()
    {
        var mappings = JArray.Parse(ReadFixture("tower-mappings.json"));
        Equal(9, mappings.Count, "mapping fixture count");

        foreach (var token in mappings)
        {
            var mapping = LandmarkShaper.RequireObject(token, "tower mapping fixture");
            var bossType = LandmarkShaper.RequireString(mapping["bossType"], "tower mapping bossType");
            var regionId = LandmarkShaper.RequireString(mapping["regionId"], "tower mapping regionId");
            var textRow = LandmarkShaper.RequireString(mapping["textRow"], "tower mapping textRow");
            var fixture = LoadObjectFixture("tower.json");
            ObjectAt(fixture, "placement")["bossType"] = $"EPalBossType::{bossType}";
            var originalParameters = ObjectAt(fixture, "bossParameters");
            var parameter = originalParameters.Properties().Single().Value.DeepClone();
            fixture["bossParameters"] = new JObject { [bossType] = parameter };
            fixture["regionRows"] = new JObject
            {
                [textRow] = new JObject
                {
                    ["TextData"] = new JObject { ["LocalizedString"] = $"Region for {bossType}" }
                }
            };

            var location = ShapeTower(ReadTowerFixture(fixture));
            Equal($"tower:{regionId}", location.Id, $"{bossType} region ID");
            Equal($"Region for {bossType} · Electric", location.Detail, $"{bossType} text-row join");
        }
    }

    private static void TowerMissingJoinsFailClosed()
    {
        var unmapped = ReadTowerFixture(LoadObjectFixture("tower.json")) with
        {
            Placement = new TowerPlacement("unknown", "EPalBossType::NewBoss", 1, 2)
        };
        Throws(() => ShapeTower(unmapped), "Unmapped placed tower BossType NewBoss");

        var missingParameters = ReadTowerFixture(LoadObjectFixture("tower.json")) with
        {
            BossParameters = new Dictionary<string, BossParameter>(StringComparer.Ordinal)
        };
        Throws(() => ShapeTower(missingParameters), "no Normal difficulty parameters");

        var missingMonster = ReadTowerFixture(LoadObjectFixture("tower.json"));
        missingMonster.MonsterRows.Remove("GYM_ElecPanda");
        Throws(() => ShapeTower(missingMonster), "has no row named GYM_ElecPanda");

        var missingPalName = ReadTowerFixture(LoadObjectFixture("tower.json"));
        missingPalName.PalNameRows.Remove("PAL_NAME_GYM_ElecPanda");
        Throws(() => ShapeTower(missingPalName), "has no row named PAL_NAME_GYM_ElecPanda");

        var missingRegion = ReadTowerFixture(LoadObjectFixture("tower.json"));
        missingRegion.RegionRows.Remove("REGION_Grass_Boss");
        Throws(() => ShapeTower(missingRegion), "has no row named REGION_Grass_Boss");
    }

    private static void TowerInvalidDataFailsClosed()
    {
        var wrongRole = ReadTowerFixture(LoadObjectFixture("tower.json"));
        ObjectAt(wrongRole.MonsterRows, "GYM_ElecPanda")["IsTowerBoss"] = false;
        Throws(() => ShapeTower(wrongRole), "is not marked as a tower boss");

        var missingRole = ReadTowerFixture(LoadObjectFixture("tower.json"));
        ObjectAt(missingRole.MonsterRows, "GYM_ElecPanda").Remove("IsTowerBoss");
        Throws(() => ShapeTower(missingRole), ".IsTowerBoss");

        var invalidCoordinates = ReadTowerFixture(LoadObjectFixture("tower.json")) with
        {
            Placement = new TowerPlacement("bad", "GrassBoss", double.PositiveInfinity, 2)
        };
        Throws(() => ShapeTower(invalidCoordinates), "non-finite coordinates");

        var missingElement = ReadTowerFixture(LoadObjectFixture("tower.json"));
        ObjectAt(missingElement.MonsterRows, "GYM_ElecPanda").Remove("ElementType1");
        Throws(() => ShapeTower(missingElement), "monster.ElementType1");
    }

    private static void DuplicateIdsFailClosed()
    {
        var alpha = ShapeAlpha(LoadObjectFixture("alpha.json"))
            ?? throw new InvalidOperationException("Expected a shaped Alpha.");
        var tower = ShapeTower(ReadTowerFixture(LoadObjectFixture("tower.json")));
        LandmarkShaper.AssertUniqueLocationIds([alpha.Location, tower]);
        Throws(
            () => LandmarkShaper.AssertUniqueLocationIds([alpha.Location, alpha.Location with { Name = "duplicate" }]),
            "Duplicate generated landmark IDs");
    }

    private static void GameVersionIsExtracted()
    {
        var version = GameVersionExtractor.Parse(ReadFixture("game-version.ini"));
        Equal("1.0.1.100619", version, "ProjectVersion");
    }

    private static void GameVersionAcceptsBomAndCrlf()
    {
        var contents = "\uFEFF" + ReadFixture("game-version.ini").Replace("\n", "\r\n", StringComparison.Ordinal);
        Equal("1.0.1.100619", GameVersionExtractor.Parse(contents), "BOM/CRLF ProjectVersion");
    }

    private static void GameVersionRequiresOneSection()
    {
        Throws(
            () => GameVersionExtractor.Parse("[Other]\nProjectVersion=1.0.1.100619\n"),
            "Expected exactly one");
        var duplicated = ReadFixture("game-version.ini") + ReadFixture("game-version.ini");
        Throws(() => GameVersionExtractor.Parse(duplicated), "found 2");
    }

    private static void GameVersionRejectsMalformedSection()
    {
        const string malformed = """
            [/Script/EngineSettings.GeneralProjectSettings]
            ProjectID=test
            [Other
            ProjectVersion=1.0.1.100619
            """;
        Throws(() => GameVersionExtractor.Parse(malformed), "Malformed INI section header");
    }

    private static void GameVersionRequiresOneKey()
    {
        var missing = "[/Script/EngineSettings.GeneralProjectSettings]\nProjectID=test\n";
        Throws(() => GameVersionExtractor.Parse(missing), "found 0");
        var duplicated = ReadFixture("game-version.ini") + "ProjectVersion=1.0.1.100619\n";
        Throws(() => GameVersionExtractor.Parse(duplicated), "found 2");
    }

    private static void GameVersionRejectsInvalidLabels()
    {
        foreach (var invalid in new[] { "1.0", "latest", "v1.0.1", "1.0.1-beta", "1.0.1.2.3" })
        {
            var contents = ReadFixture("game-version.ini").Replace("1.0.1.100619", invalid, StringComparison.Ordinal);
            Throws(() => GameVersionExtractor.Parse(contents), "complete three- or four-component numeric release");
        }
    }

    private static void SourceSnapshotsDetectChanges() => WithTemporaryDirectory(root =>
    {
        var mappingsPath = Path.Combine(root, "palworld.usmap");
        File.WriteAllText(mappingsPath, "first mapping");
        var initial = SourceSnapshots.CaptureFile(mappingsPath);
        File.WriteAllText(mappingsPath, "other mapping");
        var final = SourceSnapshots.CaptureFile(mappingsPath);

        Throws(
            () => SourceSnapshots.EnsureUnchanged("Palworld mappings file", [initial], [final]),
            "Palworld mappings file changed during export");
    });

    private static void StagedOutputsPromoteAcrossDirectories() => WithTemporaryDirectory(root =>
    {
        var mapDestination = Path.Combine(root, "maps");
        var landmarkDestination = Path.Combine(root, "landmarks");
        Directory.CreateDirectory(mapDestination);
        Directory.CreateDirectory(landmarkDestination);
        File.WriteAllText(Path.Combine(mapDestination, "manifest.json"), "old map manifest");
        File.WriteAllText(Path.Combine(mapDestination, "unrelated.txt"), "keep me");
        File.WriteAllText(Path.Combine(landmarkDestination, "manifest.json"), "old landmark manifest");

        string mapStagingRoot;
        string landmarkStagingRoot;
        using (var mapOutput = StagedOutputDirectory.Create(mapDestination))
        using (var landmarkOutput = StagedOutputDirectory.Create(landmarkDestination))
        {
            mapStagingRoot = mapOutput.StagingRoot;
            landmarkStagingRoot = landmarkOutput.StagingRoot;
            File.WriteAllText(Path.Combine(mapOutput.PayloadDirectory, "palpagos.jpg"), "map bytes");
            File.WriteAllText(Path.Combine(mapOutput.PayloadDirectory, "manifest.json"), "new map manifest");
            File.WriteAllText(Path.Combine(landmarkOutput.PayloadDirectory, "manifest.json"), "new landmark manifest");

            OutputPromotion.Promote(mapOutput, landmarkOutput);
        }

        Equal("map bytes", File.ReadAllText(Path.Combine(mapDestination, "palpagos.jpg")), "promoted map");
        Equal("new map manifest", File.ReadAllText(Path.Combine(mapDestination, "manifest.json")), "promoted map manifest");
        Equal("new landmark manifest", File.ReadAllText(Path.Combine(landmarkDestination, "manifest.json")), "promoted landmark manifest");
        Equal("keep me", File.ReadAllText(Path.Combine(mapDestination, "unrelated.txt")), "unrelated output");
        Equal(false, Directory.Exists(mapStagingRoot), "map staging cleanup");
        Equal(false, Directory.Exists(landmarkStagingRoot), "landmark staging cleanup");
    });

    private static void SourceFailurePreservesPriorOutputs() => WithTemporaryDirectory(root =>
    {
        var destination = Path.Combine(root, "maps");
        var sourcePath = Path.Combine(root, "palworld.usmap");
        Directory.CreateDirectory(destination);
        File.WriteAllText(Path.Combine(destination, "manifest.json"), "known-good manifest");
        File.WriteAllText(sourcePath, "mapping before");
        var initial = SourceSnapshots.CaptureFile(sourcePath);

        string stagingRoot;
        using (var output = StagedOutputDirectory.Create(destination))
        {
            stagingRoot = output.StagingRoot;
            File.WriteAllText(Path.Combine(output.PayloadDirectory, "manifest.json"), "unverified manifest");
            File.WriteAllText(sourcePath, "mapping after!");
            var final = SourceSnapshots.CaptureFile(sourcePath);
            Throws(
                () => SourceSnapshots.EnsureUnchanged("Palworld mappings file", [initial], [final]),
                "Palworld mappings file changed during export");

            Equal("known-good manifest", File.ReadAllText(Path.Combine(destination, "manifest.json")), "output before cleanup");
        }

        Equal("known-good manifest", File.ReadAllText(Path.Combine(destination, "manifest.json")), "output after cleanup");
        Equal(false, Directory.Exists(stagingRoot), "failed export staging cleanup");
    });

    private static void DuplicatePromotionTargetsPreservePriorOutputs() => WithTemporaryDirectory(root =>
    {
        var destination = Path.Combine(root, "combined");
        Directory.CreateDirectory(destination);
        File.WriteAllText(Path.Combine(destination, "manifest.json"), "known-good manifest");

        using var mapOutput = StagedOutputDirectory.Create(destination);
        using var landmarkOutput = StagedOutputDirectory.Create(destination);
        File.WriteAllText(Path.Combine(mapOutput.PayloadDirectory, "manifest.json"), "map manifest");
        File.WriteAllText(Path.Combine(landmarkOutput.PayloadDirectory, "manifest.json"), "landmark manifest");

        Throws(
            () => OutputPromotion.Promote(mapOutput, landmarkOutput),
            "duplicate destination files");
        Equal("known-good manifest", File.ReadAllText(Path.Combine(destination, "manifest.json")), "duplicate target output");
    });

    private static ShapedAlpha? ShapeAlpha(JObject fixture) => LandmarkShaper.ShapeAlpha(
        LandmarkShaper.RequireString(fixture["rowName"], "alpha fixture.rowName"),
        AlphaRow(fixture),
        ObjectAt(fixture, "monsterRows"),
        ObjectAt(fixture, "palNameRows"));

    private static JObject AlphaRow(JObject fixture) => ObjectAt(fixture, "row");

    private static JObject AlphaMonster(JObject fixture) =>
        ObjectAt(ObjectAt(fixture, "monsterRows"), "Penguin_Emperor");

    private static TowerFixture ReadTowerFixture(JObject fixture)
    {
        var placement = ObjectAt(fixture, "placement");
        var parameters = ObjectAt(fixture, "bossParameters").Properties().ToDictionary(
            property => property.Name,
            property =>
            {
                var value = LandmarkShaper.RequireObject(property.Value, $"bossParameters.{property.Name}");
                return new BossParameter(
                    LandmarkShaper.RequireString(value["palId"], $"bossParameters.{property.Name}.palId"),
                    LandmarkShaper.RequireInt(value["level"], $"bossParameters.{property.Name}.level"));
            },
            StringComparer.Ordinal);

        return new TowerFixture(
            new TowerPlacement(
                LandmarkShaper.RequireString(placement["actorName"], "placement.actorName"),
                LandmarkShaper.RequireString(placement["bossType"], "placement.bossType"),
                placement.Value<double>("x"),
                placement.Value<double>("y")),
            parameters,
            ObjectAt(fixture, "monsterRows"),
            ObjectAt(fixture, "palNameRows"),
            ObjectAt(fixture, "regionRows"));
    }

    private static LandmarkLocation ShapeTower(TowerFixture fixture) => LandmarkShaper.ShapeTower(
        fixture.Placement,
        fixture.MonsterRows,
        fixture.PalNameRows,
        fixture.RegionRows,
        fixture.BossParameters);

    private static JObject LoadObjectFixture(string fileName) => JObject.Parse(ReadFixture(fileName));

    private static string ReadFixture(string fileName) =>
        File.ReadAllText(Path.Combine(AppContext.BaseDirectory, "Fixtures", fileName));

    private static JObject ObjectAt(JObject parent, string property) =>
        LandmarkShaper.RequireObject(parent[property], property);

    private static void Equal(object? expected, object? actual, string context)
    {
        if (!Equals(expected, actual))
        {
            throw new InvalidOperationException($"{context}: expected <{expected ?? "null"}>, got <{actual ?? "null"}>.");
        }
    }

    private static void Close(double expected, double actual, string context)
    {
        if (Math.Abs(expected - actual) > 0.000001)
        {
            throw new InvalidOperationException($"{context}: expected {expected:R}, got {actual:R}.");
        }
    }

    private static void WithTemporaryDirectory(Action<string> action)
    {
        var path = Path.Combine(Path.GetTempPath(), $"palworld-asset-exporter-tests-{Guid.NewGuid():N}");
        Directory.CreateDirectory(path);
        try
        {
            action(path);
        }
        finally
        {
            if (Directory.Exists(path))
            {
                Directory.Delete(path, recursive: true);
            }
        }
    }

    private static void Throws(Action action, string messageFragment)
    {
        try
        {
            action();
        }
        catch (InvalidOperationException exception) when (exception.Message.Contains(messageFragment, StringComparison.Ordinal))
        {
            return;
        }
        catch (Exception exception)
        {
            throw new InvalidOperationException(
                $"Expected InvalidOperationException containing <{messageFragment}>, got {exception.GetType().Name}: {exception.Message}",
                exception);
        }
        throw new InvalidOperationException($"Expected InvalidOperationException containing <{messageFragment}>, but no exception was thrown.");
    }

    private sealed record TowerFixture(
        TowerPlacement Placement,
        IReadOnlyDictionary<string, BossParameter> BossParameters,
        JObject MonsterRows,
        JObject PalNameRows,
        JObject RegionRows);
}
