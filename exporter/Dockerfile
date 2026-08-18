# Build the exporter and its managed dependencies.
FROM mcr.microsoft.com/dotnet/sdk:10.0.302-noble@sha256:72dd743782f2ae7e5476fd64f6a460045e3998dc862218b80e6944cba79a01b0 AS build

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
FROM mcr.microsoft.com/dotnet/runtime:10.0.11-noble@sha256:acad02eb5c4fbf57d15296f9c08d56cd4036e915bdae5b4dd48a06523d452617

# Bake in the pinned Palworld community mappings file so running the exporter needs
# nothing but Docker. The build fails unless the download matches the pinned checksum.
ARG MAPPINGS_COMMIT=42cf396e714c166f17950a9c964583e0cadf2a15
ARG MAPPINGS_SHA256=241c45de9d5b55b246cd4b39d62b9209faf7758ce0637e1f7a545aa0f75f71f0
ADD https://raw.githubusercontent.com/PalworldModding/UsefulFiles/${MAPPINGS_COMMIT}/Mappings.usmap /mappings.usmap
RUN printf '%s  /mappings.usmap\n' "${MAPPINGS_SHA256}" | sha256sum -c -

WORKDIR /app
COPY --from=build /out ./

ENTRYPOINT ["dotnet", "PalworldAssetExporter.dll"]
