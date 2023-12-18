use chrono::Local;
use sea_orm::{ActiveModelTrait, ColumnTrait, DbConn, DbErr, DeleteResult, EntityTrait, PaginatorTrait, QueryFilter, QueryOrder};
use sea_orm::ActiveValue::Set;
use crate::entity::user_info;
use crate::entity::user_info::Entity;

pub struct Query;

impl Query {
    pub async fn find_user_info_by_id(db: &DbConn, id: i64) -> Result<Option<user_info::Model>, DbErr> {
        Entity::find_by_id(id).one(db).await
    }

    pub async fn login(db: &DbConn, email: &str, password: &str) -> Result<Option<user_info::Model>, DbErr> {
        Entity::find()
            .filter(user_info::Column::Email.eq(email))
            .filter(user_info::Column::Password.eq(password))
            .one(db)
            .await
    }

    pub async fn find_user_infos(db: &DbConn) -> Result<Vec<user_info::Model>, DbErr> {
        Entity::find().all(db).await
    }

    pub async fn find_user_infos_in_page(
        db: &DbConn,
        page: u64,
        posts_per_page: u64,
    ) -> Result<(Vec<user_info::Model>, u64), DbErr> {
        // Setup paginator
        let paginator = Entity::find()
            .order_by_asc(user_info::Column::Uid)
            .paginate(db, posts_per_page);
        let num_pages = paginator.num_pages().await?;

        // Fetch paginated posts
        paginator.fetch_page(page - 1).await.map(|p| (p, num_pages))
    }
}

pub struct Mutation;

impl Mutation {
    pub async fn create_user(
        db: &DbConn,
        form_data: user_info::Model,
    ) -> Result<user_info::ActiveModel, DbErr> {
        user_info::ActiveModel {
            uid: Default::default(),
            email: Set(form_data.email.to_owned()),
            nickname: Set(form_data.nickname.to_owned()),
            avatar_url: Set(form_data.avatar_url.to_owned()),
            color_schema: Set(form_data.color_schema.to_owned()),
            list_style: Set(form_data.list_style.to_owned()),
            language: Set(form_data.language.to_owned()),
            password: Set(form_data.password.to_owned()),
            created_at: Set(Local::now().date_naive()),
            updated_at: Set(Local::now().date_naive()),
        }
            .save(db)
            .await
    }

    pub async fn update_tag_by_id(
        db: &DbConn,
        id: i64,
        form_data: user_info::Model,
    ) -> Result<user_info::Model, DbErr> {
        let user: user_info::ActiveModel = Entity::find_by_id(id)
            .one(db)
            .await?
            .ok_or(DbErr::Custom("Cannot find tag.".to_owned()))
            .map(Into::into)?;

        user_info::ActiveModel {
            uid: user.uid,
            email: Set(form_data.email.to_owned()),
            nickname: Set(form_data.nickname.to_owned()),
            avatar_url: Set(form_data.avatar_url.to_owned()),
            color_schema: Set(form_data.color_schema.to_owned()),
            list_style: Set(form_data.list_style.to_owned()),
            language: Set(form_data.language.to_owned()),
            password: Set(form_data.password.to_owned()),
            created_at: Default::default(),
            updated_at: Set(Local::now().date_naive()),
        }
            .update(db)
            .await
    }

    pub async fn delete_tag(db: &DbConn, tid: i64) -> Result<DeleteResult, DbErr> {
        let tag: user_info::ActiveModel = Entity::find_by_id(tid)
            .one(db)
            .await?
            .ok_or(DbErr::Custom("Cannot find tag.".to_owned()))
            .map(Into::into)?;

        tag.delete(db).await
    }

    pub async fn delete_all_tags(db: &DbConn) -> Result<DeleteResult, DbErr> {
        Entity::delete_many().exec(db).await
    }
}