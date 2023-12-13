use sea_orm::{DbConn, DbErr, EntityTrait, PaginatorTrait, QueryOrder};
use crate::entity::dialog_info;
use crate::entity::dialog_info::Entity;

pub struct Query;

impl Query {
    pub async fn find_dialog_info_by_id(db: &DbConn, id: i64) -> Result<Option<dialog_info::Model>, DbErr> {
        Entity::find_by_id(id).one(db).await
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