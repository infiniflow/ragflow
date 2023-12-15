use chrono::{Local, NaiveDate};
use sea_orm::{ActiveModelTrait, ColumnTrait, DbConn, DbErr, DeleteResult, EntityTrait, PaginatorTrait, QueryFilter, QueryOrder};
use sea_orm::ActiveValue::Set;
use crate::entity::kb_info;
use crate::entity::kb_info::Entity;

pub struct Query;

impl Query {
    pub async fn find_kb_info_by_id(db: &DbConn, id: i64) -> Result<Option<kb_info::Model>, DbErr> {
        Entity::find_by_id(id).one(db).await
    }

    pub async fn find_kb_infos(db: &DbConn) -> Result<Vec<kb_info::Model>, DbErr> {
        Entity::find().all(db).await
    }

    pub async fn find_kb_infos_by_uid(db: &DbConn, uid: i64) -> Result<Vec<kb_info::Model>, DbErr> {
        Entity::find()
            .filter(kb_info::Column::Uid.eq(uid))
            .all(db)
            .await
    }

    pub async fn find_kb_infos_in_page(
        db: &DbConn,
        page: u64,
        posts_per_page: u64,
    ) -> Result<(Vec<kb_info::Model>, u64), DbErr> {
        // Setup paginator
        let paginator = Entity::find()
            .order_by_asc(kb_info::Column::KbId)
            .paginate(db, posts_per_page);
        let num_pages = paginator.num_pages().await?;

        // Fetch paginated posts
        paginator.fetch_page(page - 1).await.map(|p| (p, num_pages))
    }
}

pub struct Mutation;

impl Mutation {
    pub async fn create_kb_info(
        db: &DbConn,
        form_data: kb_info::Model,
    ) -> Result<kb_info::ActiveModel, DbErr> {
        kb_info::ActiveModel {
            kb_id: Default::default(),
            uid: Set(form_data.uid.to_owned()),
            kn_name: Set(form_data.kn_name.to_owned()),
            icon: Set(form_data.icon.to_owned()),
            created_at: Set(Local::now().date_naive()),
            updated_at: Set(Local::now().date_naive()),
        }
            .save(db)
            .await
    }

    pub async fn update_kb_info_by_id(
        db: &DbConn,
        id: i64,
        form_data: kb_info::Model,
    ) -> Result<kb_info::Model, DbErr> {
        let kb_info: kb_info::ActiveModel = Entity::find_by_id(id)
            .one(db)
            .await?
            .ok_or(DbErr::Custom("Cannot find.".to_owned()))
            .map(Into::into)?;

        kb_info::ActiveModel {
            kb_id: kb_info.kb_id,
            uid: kb_info.uid,
            kn_name: Set(form_data.kn_name.to_owned()),
            icon: Set(form_data.icon.to_owned()),
            created_at: Default::default(),
            updated_at: Set(Local::now().date_naive()),
        }
            .update(db)
            .await
    }

    pub async fn delete_kb_info(db: &DbConn, kb_id: i64) -> Result<DeleteResult, DbErr> {
        let tag: kb_info::ActiveModel = Entity::find_by_id(kb_id)
            .one(db)
            .await?
            .ok_or(DbErr::Custom("Cannot find.".to_owned()))
            .map(Into::into)?;

        tag.delete(db).await
    }

    pub async fn delete_all_kb_infos(db: &DbConn) -> Result<DeleteResult, DbErr> {
        Entity::delete_many().exec(db).await
    }
}