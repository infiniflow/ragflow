use sea_orm_migration::prelude::*;

#[derive(DeriveMigrationName)]
pub struct Migration;

#[async_trait::async_trait]
impl MigrationTrait for Migration {
    async fn up(&self, manager: &SchemaManager) -> Result<(), DbErr> {
        manager
            .create_table(
                Table::create()
                    .table(UserInfo::Table)
                    .if_not_exists()
                    .col(
                        ColumnDef::new(UserInfo::Uid)
                            .big_integer()
                            .not_null()
                            .auto_increment()
                            .primary_key(),
                    )
                    .col(ColumnDef::new(UserInfo::Email).string().not_null())
                    .col(ColumnDef::new(UserInfo::Nickname).string().not_null())
                    .col(ColumnDef::new(UserInfo::AvatarUrl).string())
                    .col(ColumnDef::new(UserInfo::ColorSchema).string().default("dark"))
                    .col(ColumnDef::new(UserInfo::ListStyle).string().default("list"))
                    .col(ColumnDef::new(UserInfo::Language).string().default("chinese"))
                    .col(ColumnDef::new(UserInfo::CreatedAt).date().not_null())
                    .col(ColumnDef::new(UserInfo::UpdatedAt).date().not_null())
                    .col(ColumnDef::new(UserInfo::IsDeleted).boolean().default(false))
                    .to_owned(),
            )
            .await?;

        manager
            .create_table(
                Table::create()
                    .table(TagInfo::Table)
                    .if_not_exists()
                    .col(
                        ColumnDef::new(TagInfo::Tid)
                            .big_integer()
                            .not_null()
                            .auto_increment()
                            .primary_key(),
                    )
                    .col(ColumnDef::new(TagInfo::Uid).big_integer().not_null())
                    .col(ColumnDef::new(TagInfo::TagName).string().not_null())
                    .col(ColumnDef::new(TagInfo::Regx).string())
                    .col(ColumnDef::new(TagInfo::Color).big_integer().default(1))
                    .col(ColumnDef::new(TagInfo::Icon).big_integer().default(1))
                    .col(ColumnDef::new(TagInfo::Dir).string())
                    .col(ColumnDef::new(TagInfo::CreatedAt).date().not_null())
                    .col(ColumnDef::new(TagInfo::UpdatedAt).date().not_null())
                    .col(ColumnDef::new(TagInfo::IsDeleted).boolean().default(false))
                    .to_owned(),
            )
            .await?;

        manager
            .create_table(
                Table::create()
                    .table(Tag2Doc::Table)
                    .if_not_exists()
                    .col(ColumnDef::new(Tag2Doc::TagId).big_integer())
                    .col(ColumnDef::new(Tag2Doc::Did).big_integer())
                    .to_owned(),
            )
            .await?;

        manager
            .create_table(
                Table::create()
                    .table(Kb2Doc::Table)
                    .if_not_exists()
                    .col(ColumnDef::new(Kb2Doc::KbId).big_integer())
                    .col(ColumnDef::new(Kb2Doc::Did).big_integer())
                    .to_owned(),
            )
            .await?;

        manager
            .create_table(
                Table::create()
                    .table(Dialog2Kb::Table)
                    .if_not_exists()
                    .col(ColumnDef::new(Dialog2Kb::DialogId).big_integer())
                    .col(ColumnDef::new(Dialog2Kb::KbId).big_integer())
                    .to_owned(),
            )
            .await?;

        manager
            .create_table(
                Table::create()
                    .table(Doc2Doc::Table)
                    .if_not_exists()
                    .col(ColumnDef::new(Doc2Doc::ParentId).big_integer())
                    .col(ColumnDef::new(Doc2Doc::Did).big_integer())
                    .to_owned(),
            )
            .await?;

        manager
            .create_table(
                Table::create()
                    .table(KbInfo::Table)
                    .if_not_exists()
                    .col(ColumnDef::new(KbInfo::KbId).big_integer()
                        .auto_increment()
                        .not_null()
                        .primary_key())
                    .col(ColumnDef::new(KbInfo::Uid).big_integer().not_null())
                    .col(ColumnDef::new(KbInfo::KbName).string().not_null())
                    .col(ColumnDef::new(KbInfo::Icon).big_integer().default(1))
                    .col(ColumnDef::new(KbInfo::CreatedAt).date().not_null())
                    .col(ColumnDef::new(KbInfo::UpdatedAt).date().not_null())
                    .col(ColumnDef::new(KbInfo::IsDeleted).boolean().default(false))
                    .to_owned(),
            )
            .await?;

        manager
            .create_table(
                Table::create()
                    .table(DocInfo::Table)
                    .if_not_exists()
                    .col(ColumnDef::new(DocInfo::Did).big_integer()
                        .not_null()
                        .auto_increment()
                        .primary_key())
                    .col(ColumnDef::new(DocInfo::Uid).big_integer().not_null())
                    .col(ColumnDef::new(DocInfo::DocName).string().not_null())
                    .col(ColumnDef::new(DocInfo::Location).string().not_null())
                    .col(ColumnDef::new(DocInfo::Size).big_integer().not_null())
                    .col(ColumnDef::new(DocInfo::Type).string().not_null()).comment("doc|folder")
                    .col(ColumnDef::new(DocInfo::KbProgress).float().default(0))
                    .col(ColumnDef::new(DocInfo::CreatedAt).date().not_null())
                    .col(ColumnDef::new(DocInfo::UpdatedAt).date().not_null())
                    .col(ColumnDef::new(DocInfo::IsDeleted).boolean().default(false))
                    .to_owned(),
            )
            .await?;

        manager
            .create_table(
                Table::create()
                    .table(DialogInfo::Table)
                    .if_not_exists()
                    .col(ColumnDef::new(DialogInfo::DialogId)
                        .big_integer()
                        .not_null()
                        .auto_increment()
                        .primary_key())
                    .col(ColumnDef::new(DialogInfo::Uid).big_integer().not_null())
                    .col(ColumnDef::new(DialogInfo::DialogName).string().not_null())
                    .col(ColumnDef::new(DialogInfo::History).string().comment("json"))
                    .col(ColumnDef::new(DialogInfo::CreatedAt).date().not_null())
                    .col(ColumnDef::new(DialogInfo::UpdatedAt).date().not_null())
                    .col(ColumnDef::new(DialogInfo::IsDeleted).boolean().default(false))
                    .to_owned(),
            )
            .await?;

        Ok(())
    }

    async fn down(&self, manager: &SchemaManager) -> Result<(), DbErr> {
        manager
            .drop_table(Table::drop().table(UserInfo::Table).to_owned())
            .await?;

        manager
            .drop_table(Table::drop().table(TagInfo::Table).to_owned())
            .await?;

        manager
            .drop_table(Table::drop().table(Tag2Doc::Table).to_owned())
            .await?;

        manager
            .drop_table(Table::drop().table(Kb2Doc::Table).to_owned())
            .await?;

        manager
            .drop_table(Table::drop().table(Dialog2Kb::Table).to_owned())
            .await?;

        manager
            .drop_table(Table::drop().table(Doc2Doc::Table).to_owned())
            .await?;

        manager
            .drop_table(Table::drop().table(KbInfo::Table).to_owned())
            .await?;

        manager
            .drop_table(Table::drop().table(DocInfo::Table).to_owned())
            .await?;

        manager
            .drop_table(Table::drop().table(DialogInfo::Table).to_owned())
            .await?;

        Ok(())
    }
}

#[derive(DeriveIden)]
enum UserInfo {
    Table,
    Uid,
    Email,
    Nickname,
    AvatarUrl,
    ColorSchema,
    ListStyle,
    Language,
    CreatedAt,
    UpdatedAt,
    IsDeleted,
}

#[derive(DeriveIden)]
enum TagInfo {
    Table,
    Tid,
    Uid,
    TagName,
    Regx,
    Color,
    Icon,
    Dir,
    CreatedAt,
    UpdatedAt,
    IsDeleted,
}

#[derive(DeriveIden)]
enum Tag2Doc {
    Table,
    TagId,
    Did,
}

#[derive(DeriveIden)]
enum Kb2Doc {
    Table,
    KbId,
    Did,
}

#[derive(DeriveIden)]
enum Dialog2Kb {
    Table,
    DialogId,
    KbId,
}

#[derive(DeriveIden)]
enum Doc2Doc {
    Table,
    ParentId,
    Did,
}

#[derive(DeriveIden)]
enum KbInfo {
    Table,
    KbId,
    Uid,
    KbName,
    Icon,
    CreatedAt,
    UpdatedAt,
    IsDeleted,
}

#[derive(DeriveIden)]
enum DocInfo {
    Table,
    Did,
    Uid,
    DocName,
    Location,
    Size,
    Type,
    KbProgress,
    CreatedAt,
    UpdatedAt,
    IsDeleted,
}

#[derive(DeriveIden)]
enum DialogInfo {
    Table,
    DialogId,
    Uid,
    DialogName,
    History,
    CreatedAt,
    UpdatedAt,
    IsDeleted,
}