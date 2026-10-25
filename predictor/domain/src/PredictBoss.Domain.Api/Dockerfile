FROM mcr.microsoft.com/dotnet/sdk:10.0 AS build
WORKDIR /src
COPY predictor/domain/src/ ./predictor/domain/src/
RUN dotnet publish predictor/domain/src/PredictBoss.Domain.Api/PredictBoss.Domain.Api.csproj -c Release -o /app/publish

FROM mcr.microsoft.com/dotnet/aspnet:10.0
WORKDIR /app
COPY --from=build /app/publish .
ENV ASPNETCORE_URLS=http://+:8080
EXPOSE 8080
ENTRYPOINT ["dotnet", "PredictBoss.Domain.Api.dll"]
