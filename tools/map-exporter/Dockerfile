FROM mcr.microsoft.com/dotnet/sdk:10.0 AS build
WORKDIR /src
COPY MapExporter.csproj ./
RUN dotnet restore
COPY Program.cs ./
RUN dotnet publish -c Release --no-restore -o /out

FROM mcr.microsoft.com/dotnet/runtime:10.0
WORKDIR /app
COPY --from=build /out ./
ENTRYPOINT ["dotnet", "MapExporter.dll"]

