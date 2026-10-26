using Microsoft.OpenApi;
using Swashbuckle.AspNetCore.SwaggerGen;

public sealed class SwaggerSecurityOperationFilter : IOperationFilter
{
    public void Apply(OpenApiOperation operation, OperationFilterContext context)
    {
        var path = context.ApiDescription.RelativePath ?? "";
        var schemeName = path.StartsWith("api/v1/", StringComparison.OrdinalIgnoreCase) ? "DomainJwt" : null;
        if (schemeName is null) return;

        operation.Security =
        [
            new OpenApiSecurityRequirement
            {
                [new OpenApiSecuritySchemeReference(schemeName, context.Document)] = []
            }
        ];
    }
}
