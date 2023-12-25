use chrono::{ FixedOffset, Utc };
use sea_orm::{
    ActiveModelTrait,
    DbConn,
    DbErr,
    DeleteResult,
    EntityTrait,
    PaginatorTrait,
    QueryOrder,
    ColumnTrait,
    QueryFilter,
};
use sea_orm::ActiveValue::{ Set, NotSet };
use crate::entity::tag_info;
use crate::entity::tag_info::Entity;

fn now() -> chrono::DateTime<FixedOffset> {
    Utc::now().with_timezone(&FixedOffset::east_opt(3600 * 8).unwrap())
}
pub struct Query;

impl Query {
    pub async fn find_tag_info_by_id(
        id: i64,
        db: &DbConn
    ) -> Result<Option<tag_info::Model>, DbErr> {
        Entity::find_by_id(id).one(db).await
    }

    pub async fn find_tags_by_uid(uid: i64, db: &DbConn) -> Result<Vec<tag_info::Model>, DbErr> {
        Entity::find().filter(tag_info::Column::Uid.eq(uid)).all(db).await
    }

    pub async fn find_tag_infos_in_page(
        db: &DbConn,
        page: u64,
        posts_per_page: u64
    ) -> Result<(Vec<tag_info::Model>, u64), DbErr> {
        // Setup paginator
        let paginator = Entity::find()
            .order_by_asc(tag_info::Column::Tid)
            .paginate(db, posts_per_page);
        let num_pages = paginator.num_pages().await?;

        // Fetch paginated posts
        paginator.fetch_page(page - 1).await.map(|p| (p, num_pages))
    }
}

pub struct Mutation;

impl Mutation {
    pub async fn create_tag(
        db: &DbConn,
        form_data: tag_info::Model
    ) -> Result<tag_info::ActiveModel, DbErr> {
        (tag_info::ActiveModel {
            tid: Default::default(),
            uid: Set(form_data.uid.to_owned()),
            tag_name: Set(form_data.tag_name.to_owned()),
            regx: Set(form_data.regx.to_owned()),
            color: Set(form_data.color.to_owned()),
            icon: Set(form_data.icon.to_owned()),
            folder_id: match form_data.folder_id {
                0 => NotSet,
                _ => Set(form_data.folder_id.to_owned()),
            },
            created_at: Set(now()),
            updated_at: Set(now()),
        }).save(db).await
    }

    pub async fn update_tag_by_id(
        db: &DbConn,
        id: i64,
        form_data: tag_info::Model
    ) -> Result<tag_info::Model, DbErr> {
        let tag: tag_info::ActiveModel = Entity::find_by_id(id)
            .one(db).await?
            .ok_or(DbErr::Custom("Cannot find tag.".to_owned()))
            .map(Into::into)?;

        (tag_info::ActiveModel {
            tid: tag.tid,
            uid: tag.uid,
            tag_name: Set(form_data.tag_name.to_owned()),
            regx: Set(form_data.regx.to_owned()),
            color: Set(form_data.color.to_owned()),
            icon: Set(form_data.icon.to_owned()),
            folder_id: Set(form_data.folder_id.to_owned()),
            created_at: Default::default(),
            updated_at: Set(now()),
        }).update(db).await
    }

    pub async fn delete_tag(db: &DbConn, tid: i64) -> Result<DeleteResult, DbErr> {
        let tag: tag_info::ActiveModel = Entity::find_by_id(tid)
            .one(db).await?
            .ok_or(DbErr::Custom("Cannot find tag.".to_owned()))
            .map(Into::into)?;

        tag.delete(db).await
    }

    pub async fn delete_all_tags(db: &DbConn) -> Result<DeleteResult, DbErr> {
        Entity::delete_many().exec(db).await
    }
}
