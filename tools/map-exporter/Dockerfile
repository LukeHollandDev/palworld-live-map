# Build the exporter and its managed dependencies.
FROM mcr.microsoft.com/dotnet/sdk:10.0 AS build

WORKDIR /src

# Restore separately so dependency downloads remain cached when the source changes.
COPY MapExporter.csproj ./
RUN dotnet restore

COPY Program.cs ./
RUN dotnet publish -c Release --no-restore -o /out

# Run the published exporter without the .NET SDK.
FROM mcr.microsoft.com/dotnet/runtime:10.0

WORKDIR /app
COPY --from=build /out ./

ENTRYPOINT ["dotnet", "MapExporter.dll"]
