use chrono::Local;
use sea_orm::{ActiveModelTrait, DbConn, DbErr, DeleteResult, EntityTrait, PaginatorTrait, QueryOrder};
use sea_orm::ActiveValue::Set;
use sea_orm::QueryFilter;
use sea_orm::ColumnTrait;
use crate::entity::{dialog_info, kb_info};
use crate::entity::dialog_info::Entity;

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
        form_data: dialog_info::Model,
    ) -> Result<dialog_info::ActiveModel, DbErr> {
        dialog_info::ActiveModel {
            dialog_id: Default::default(),
            uid: Set(form_data.uid.to_owned()),
            dialog_name: Set(form_data.dialog_name.to_owned()),
            history: Set(form_data.history.to_owned()),
            created_at: Set(Local::now().date_naive()),
            updated_at: Set(Local::now().date_naive()),
        }
            .save(db)
            .await
    }

    pub async fn update_dialog_info_by_id(
        db: &DbConn,
        id: i64,
        form_data: dialog_info::Model,
    ) -> Result<dialog_info::Model, DbErr> {
        let dialog_info: dialog_info::ActiveModel = Entity::find_by_id(id)
            .one(db)
            .await?
            .ok_or(DbErr::Custom("Cannot find.".to_owned()))
            .map(Into::into)?;

        dialog_info::ActiveModel {
            dialog_id: dialog_info.dialog_id,
            uid: dialog_info.uid,
            dialog_name: Set(form_data.dialog_name.to_owned()),
            history: Set(form_data.history.to_owned()),
            created_at: Default::default(),
            updated_at: Set(Local::now().date_naive()),
        }
            .update(db)
            .await
    }

    pub async fn delete_dialog_info(db: &DbConn, kb_id: i64) -> Result<DeleteResult, DbErr> {
        let tag: dialog_info::ActiveModel = Entity::find_by_id(kb_id)
            .one(db)
            .await?
            .ok_or(DbErr::Custom("Cannot find.".to_owned()))
            .map(Into::into)?;

        tag.delete(db).await
    }

    pub async fn delete_all_dialog_infos(db: &DbConn) -> Result<DeleteResult, DbErr> {
        Entity::delete_many().exec(db).await
    }
}