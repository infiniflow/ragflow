use chrono::{FixedOffset, Utc};
use migration::Expr;
use sea_orm::{ActiveModelTrait, ColumnTrait, DbConn, DbErr, DeleteResult, EntityTrait, PaginatorTrait, QueryFilter, QueryOrder, UpdateResult};
use sea_orm::ActiveValue::Set;
use crate::entity::user_info;
use crate::entity::user_info::Entity;

fn now()->chrono::DateTime<FixedOffset>{
    Utc::now().with_timezone(&FixedOffset::east_opt(3600*8).unwrap())
}
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
        form_data: &user_info::Model,
    ) -> Result<user_info::ActiveModel, DbErr> {
        user_info::ActiveModel {
            uid: Default::default(),
            email: Set(form_data.email.to_owned()),
            nickname: Set(form_data.nickname.to_owned()),
            avatar_base64: Set(form_data.avatar_base64.to_owned()),
            color_scheme: Set(form_data.color_scheme.to_owned()),
            list_style: Set(form_data.list_style.to_owned()),
            language: Set(form_data.language.to_owned()),
            password: Set(form_data.password.to_owned()),
            last_login_at: Set(now()),
            created_at: Set(now()),
            updated_at: Set(now()),
        }
            .save(db)
            .await
    }

    pub async fn update_user_by_id(
        db: &DbConn,
        form_data: &user_info::Model,
    ) -> Result<user_info::Model, DbErr> {
        let usr: user_info::ActiveModel = Entity::find_by_id(form_data.uid)
            .one(db)
            .await?
            .ok_or(DbErr::Custom("Cannot find user.".to_owned()))
            .map(Into::into)?;

        user_info::ActiveModel {
            uid: Set(form_data.uid),
            email: Set(form_data.email.to_owned()),
            nickname: Set(form_data.nickname.to_owned()),
            avatar_base64: Set(form_data.avatar_base64.to_owned()),
            color_scheme: Set(form_data.color_scheme.to_owned()),
            list_style: Set(form_data.list_style.to_owned()),
            language: Set(form_data.language.to_owned()),
            password: Set(form_data.password.to_owned()),
            updated_at: Set(now()),
            last_login_at: usr.last_login_at,
            created_at:usr.created_at,
        }
            .update(db)
            .await
    }

    pub async fn update_login_status(
        uid: i64,
        db: &DbConn
    ) -> Result<UpdateResult, DbErr> {
        Entity::update_many()
            .col_expr(user_info::Column::LastLoginAt,  Expr::value(now()))
            .filter(user_info::Column::Uid.eq(uid))
            .exec(db)
            .await
    }

    pub async fn delete_user(db: &DbConn, tid: i64) -> Result<DeleteResult, DbErr> {
        let tag: user_info::ActiveModel = Entity::find_by_id(tid)
            .one(db)
            .await?
            .ok_or(DbErr::Custom("Cannot find tag.".to_owned()))
            .map(Into::into)?;

        tag.delete(db).await
    }

    pub async fn delete_all(db: &DbConn) -> Result<DeleteResult, DbErr> {
        Entity::delete_many().exec(db).await
    }
}