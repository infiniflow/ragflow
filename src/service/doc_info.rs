use chrono::Local;
use postgres::fallible_iterator::FallibleIterator;
use sea_orm::{ActiveModelTrait, ColumnTrait, DbConn, DbErr, DeleteResult, EntityTrait, PaginatorTrait, QueryOrder};
use sea_orm::ActiveValue::Set;
use sea_orm::QueryFilter;
use crate::api::doc_info::{FilterParams, Params};
use crate::entity::{doc2_doc, doc_info, kb_info, tag_info};
use crate::entity::doc_info::Entity;

pub struct Query;

impl Query {
    pub async fn find_doc_info_by_id(db: &DbConn, id: i64) -> Result<Option<doc_info::Model>, DbErr> {
        Entity::find_by_id(id).one(db).await
    }

    pub async fn find_doc_infos(db: &DbConn) -> Result<Vec<doc_info::Model>, DbErr> {
        Entity::find().all(db).await
    }

    pub async fn find_doc_infos_by_uid(db: &DbConn, uid: i64) -> Result<Vec<doc_info::Model>, DbErr> {
        Entity::find()
            .filter(doc_info::Column::Uid.eq(uid))
            .all(db)
            .await
    }

    pub async fn find_doc_infos_by_params(db: &DbConn, params: Params) -> Result<Vec<doc_info::Model>, DbErr> {
        // Setup paginator
        let paginator = Entity::find();

        // Fetch paginated posts
        let mut query = paginator
            .find_with_related(kb_info::Entity);
        if let Some(kb_id) = params.filter.kb_id {
            query = query.filter(kb_info::Column::KbId.eq(kb_id));
        }
        if let Some(folder_id) = params.filter.folder_id {

        }
        if let Some(tag_id) = params.filter.tag_id {
            query = query.filter(tag_info::Column::Tid.eq(tag_id));
        }
        if let Some(keywords) = params.filter.keywords {

        }
        Ok(query.order_by_asc(doc_info::Column::Did)
            .all(db)
            .await?
            .into_iter()
            .map(|(mut doc_info, kb_infos)| {
                doc_info.kb_infos = kb_infos;
                doc_info
            })
            .collect())
    }

    pub async fn find_doc_infos_in_page(
        db: &DbConn,
        page: u64,
        posts_per_page: u64,
    ) -> Result<(Vec<doc_info::Model>, u64), DbErr> {
        // Setup paginator
        let paginator = Entity::find()
            .order_by_asc(doc_info::Column::Did)
            .paginate(db, posts_per_page);
        let num_pages = paginator.num_pages().await?;

        // Fetch paginated posts
        paginator.fetch_page(page - 1).await.map(|p| (p, num_pages))
    }
}

pub struct Mutation;

impl Mutation {

    pub async fn mv_doc_info(
        db: &DbConn,
        dest_did: i64,
        dids: &[i64]
    ) -> Result<(), DbErr> {
        for did in dids {
            let _ = doc2_doc::ActiveModel {
                parent_id: Set(dest_did),
                did: Set(*did),
            }
                .save(db)
                .await
                .unwrap();
        }

        Ok(())
    }

    pub async fn create_doc_info(
        db: &DbConn,
        form_data: doc_info::Model,
    ) -> Result<doc_info::ActiveModel, DbErr> {
        doc_info::ActiveModel {
            did: Default::default(),
            uid: Set(form_data.uid.to_owned()),
            doc_name: Set(form_data.doc_name.to_owned()),
            size: Set(form_data.size.to_owned()),
            r#type: Set(form_data.r#type.to_owned()),
            kb_progress: Set(form_data.kb_progress.to_owned()),
            location: Set(form_data.location.to_owned()),
            created_at: Set(Local::now().date_naive()),
            updated_at: Set(Local::now().date_naive()),
        }
            .save(db)
            .await
    }

    pub async fn update_doc_info_by_id(
        db: &DbConn,
        id: i64,
        form_data: doc_info::Model,
    ) -> Result<doc_info::Model, DbErr> {
        let doc_info: doc_info::ActiveModel = Entity::find_by_id(id)
            .one(db)
            .await?
            .ok_or(DbErr::Custom("Cannot find.".to_owned()))
            .map(Into::into)?;

        doc_info::ActiveModel {
            did: doc_info.did,
            uid: Set(form_data.uid.to_owned()),
            doc_name: Set(form_data.doc_name.to_owned()),
            size: Set(form_data.size.to_owned()),
            r#type: Set(form_data.r#type.to_owned()),
            kb_progress: Set(form_data.kb_progress.to_owned()),
            location: Set(form_data.location.to_owned()),
            created_at: Default::default(),
            updated_at: Set(Local::now().date_naive()),
        }
            .update(db)
            .await
    }

    pub async fn delete_doc_info(db: &DbConn, doc_id: i64) -> Result<DeleteResult, DbErr> {
        let tag: doc_info::ActiveModel = Entity::find_by_id(doc_id)
            .one(db)
            .await?
            .ok_or(DbErr::Custom("Cannot find.".to_owned()))
            .map(Into::into)?;

        tag.delete(db).await
    }

    pub async fn delete_all_doc_infos(db: &DbConn) -> Result<DeleteResult, DbErr> {
        Entity::delete_many().exec(db).await
    }
}