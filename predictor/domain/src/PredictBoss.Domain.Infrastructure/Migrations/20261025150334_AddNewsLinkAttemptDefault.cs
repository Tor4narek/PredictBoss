using Microsoft.EntityFrameworkCore.Migrations;

#nullable disable

namespace PredictBoss.Domain.Infrastructure.Migrations
{
    /// <inheritdoc />
    public partial class AddNewsLinkAttemptDefault : Migration
    {
        /// <inheritdoc />
        protected override void Up(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.AlterColumn<int>(
                name: "send_attempts",
                table: "news_links",
                type: "integer",
                nullable: false,
                defaultValue: 0,
                oldClrType: typeof(int),
                oldType: "integer");
        }

        /// <inheritdoc />
        protected override void Down(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.AlterColumn<int>(
                name: "send_attempts",
                table: "news_links",
                type: "integer",
                nullable: false,
                oldClrType: typeof(int),
                oldType: "integer",
                oldDefaultValue: 0);
        }
    }
}
