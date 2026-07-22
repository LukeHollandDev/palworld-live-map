# Build the exporter and its managed dependencies.
FROM mcr.microsoft.com/dotnet/sdk:10.0.302-noble@sha256:ed034a8bf0b24ded0cbbac07e17825d8e9ebfe21e308191d0f7421eaf5ad4664 AS build

WORKDIR /src

# Restore separately so dependency downloads remain cached when the source changes.
COPY global.json MapExporter.csproj packages.lock.json ./
COPY tests/MapExporter.Tests.csproj tests/packages.lock.json ./tests/
RUN dotnet restore tests/MapExporter.Tests.csproj --locked-mode

COPY Program.cs ExportInfrastructure.cs GameVersionExtractor.cs LandmarkExporter.cs LandmarkShaper.cs AssemblyInfo.cs ./
COPY tests/Program.cs ./tests/
COPY tests/Fixtures/ ./tests/Fixtures/
RUN dotnet run --project tests/MapExporter.Tests.csproj -c Release --no-restore
RUN dotnet publish MapExporter.csproj -c Release --no-restore -o /out

# Run the published exporter without the .NET SDK.
FROM mcr.microsoft.com/dotnet/runtime:10.0.10-noble@sha256:ed5d539b27842d656a06a5984dbcb5114d3e885fbada612a49a5a7c3c3a44e1c

WORKDIR /app
COPY --from=build /out ./

ENTRYPOINT ["dotnet", "MapExporter.dll"]
