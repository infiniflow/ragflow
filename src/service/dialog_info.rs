use chrono::{Local, FixedOffset, Utc};
use migration::Expr;
use sea_orm::{ActiveModelTrait, DbConn, DbErr, DeleteResult, EntityTrait, PaginatorTrait, QueryOrder, UpdateResult};
use sea_orm::ActiveValue::Set;
use sea_orm::QueryFilter;
use sea_orm::ColumnTrait;
use crate::entity::dialog_info;
use crate::entity::dialog_info::Entity;

fn now()->chrono::DateTime<FixedOffset>{
    Utc::now().with_timezone(&FixedOffset::east_opt(3600*8).unwrap())
}
pub struct Query;

impl Query {
    pub async fn find_dialog_info_by_id(db: &DbConn, id: i64) -> Result<Option<dialog_info::Model>, DbErr> {
        Entity::find_by_id(id).one(db).await
    }

    pub async fn find_dialog_infos(db: &DbConn) -> Result<Vec<dialog_info::Model>, DbErr> {
        Entity::find().all(db).await
    }

    pub async fn find_dialog_infos_by_uid(db: &DbConn, uid: i64) -> Result<Vec<dialog_info::Model>, DbErr> {
        Entity::find()
            .filter(dialog_info::Column::Uid.eq(uid))
            .filter(dialog_info::Column::IsDeleted.eq(false))
            .all(db)
            .await
    }

    pub async fn find_dialog_infos_in_page(
        db: &DbConn,
        page: u64,
        posts_per_page: u64,
    ) -> Result<(Vec<dialog_info::Model>, u64), DbErr> {
        // Setup paginator
        let paginator = Entity::find()
            .order_by_asc(dialog_info::Column::DialogId)
            .paginate(db, posts_per_page);
        let num_pages = paginator.num_pages().await?;

        // Fetch paginated posts
        paginator.fetch_page(page - 1).await.map(|p| (p, num_pages))
    }
}

pub struct Mutation;

impl Mutation {
    pub async fn create_dialog_info(
        db: &DbConn,
        uid: i64,
        kb_id: i64,
        name: &String
    ) -> Result<dialog_info::ActiveModel, DbErr> {
        dialog_info::ActiveModel {
            dialog_id: Default::default(),
            uid: Set(uid),
            kb_id: Set(kb_id),
            dialog_name: Set(name.to_owned()),
            history: Set("".to_owned()),
            created_at: Set(now()),
            updated_at: Set(now()),
            is_deleted: Default::default()
        }
            .save(db)
            .await
    }

    pub async fn update_dialog_info_by_id(
        db: &DbConn,
        dialog_id: i64,
        dialog_name:&String,
        history: &String
    ) -> Result<UpdateResult, DbErr> {
        Entity::update_many()
            .col_expr(dialog_info::Column::DialogName, Expr::value(dialog_name))
            .col_expr(dialog_info::Column::History, Expr::value(history))
            .col_expr(dialog_info::Column::UpdatedAt, Expr::value(now()))
            .filter(dialog_info::Column::DialogId.eq(dialog_id))
            .exec(db)
            .await
    }

    pub async fn delete_dialog_info(db: &DbConn, dialog_id: i64) -> Result<UpdateResult, DbErr> {
        Entity::update_many()
            .col_expr(dialog_info::Column::IsDeleted, Expr::value(true))
            .filter(dialog_info::Column::DialogId.eq(dialog_id))
            .exec(db)
            .await
    }

    pub async fn delete_all_dialog_infos(db: &DbConn) -> Result<DeleteResult, DbErr> {
        Entity::delete_many().exec(db).await
    }
}