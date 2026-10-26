FROM mcr.microsoft.com/dotnet/sdk:10.0 AS build
WORKDIR /src
COPY predictor/domain/src/ ./predictor/domain/src/
RUN dotnet publish predictor/domain/src/PredictBoss.Domain.Worker/PredictBoss.Domain.Worker.csproj -c Release -o /app/publish

FROM mcr.microsoft.com/dotnet/runtime:10.0-noble
WORKDIR /app
COPY --from=build /app/publish .
ENV PLAYWRIGHT_BROWSERS_PATH=/ms-playwright
RUN dotnet PredictBoss.Domain.Worker.dll --install-browser \
    && useradd --create-home --uid 10001 worker \
    && chmod -R a+rX /ms-playwright \
    && rm -rf /var/lib/apt/lists/*
USER worker
ENTRYPOINT ["dotnet", "PredictBoss.Domain.Worker.dll"]
