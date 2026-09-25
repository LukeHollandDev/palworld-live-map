# Build the exporter and its managed dependencies.
FROM mcr.microsoft.com/dotnet/sdk:10.0.401-noble@sha256:35d40304542c8689331f8cab17c65926cdf48fe711e289321d71924b230a7d29 AS build

WORKDIR /src

# Restore separately so dependency downloads remain cached when the source changes.
COPY global.json ./
COPY src/PalworldAssetExporter.csproj src/packages.lock.json ./src/
COPY tests/PalworldAssetExporter.Tests.csproj tests/packages.lock.json ./tests/
RUN dotnet restore tests/PalworldAssetExporter.Tests.csproj --locked-mode

COPY src/*.cs ./src/
COPY tests/*.cs ./tests/
COPY tests/Fixtures/ ./tests/Fixtures/
RUN dotnet run --project tests/PalworldAssetExporter.Tests.csproj -c Release --no-restore
RUN dotnet publish src/PalworldAssetExporter.csproj -c Release --no-restore -o /out

# Run the published exporter without the .NET SDK.
FROM mcr.microsoft.com/dotnet/runtime:10.0.12-noble@sha256:8a153b5889d796b6450295b383596b13308c24c230515f8a7770ce1b94e0c460

# Bake in the pinned Palworld community mappings file so running the exporter needs
# nothing but Docker. The build fails unless the download matches the pinned checksum.
# Updated for Palworld 1.0.4.102642.
ARG MAPPINGS_COMMIT=0e4ae19a05ba0d9fb95d859c09b28f168cb3624f
ARG MAPPINGS_SHA256=604550ba90faab1e394c2789f38eeff625493d3729c2d7f6a6058bfedb90a67b
ADD https://raw.githubusercontent.com/PalworldModding/UsefulFiles/${MAPPINGS_COMMIT}/Mappings.usmap /mappings.usmap
RUN printf '%s  /mappings.usmap\n' "${MAPPINGS_SHA256}" | sha256sum -c -

WORKDIR /app
COPY --from=build /out ./

ENTRYPOINT ["dotnet", "PalworldAssetExporter.dll"]
