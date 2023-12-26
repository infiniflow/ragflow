use chrono::{ Utc, FixedOffset };
use sea_orm::{
    ActiveModelTrait,
    ColumnTrait,
    DbConn,
    DbErr,
    DeleteResult,
    EntityTrait,
    PaginatorTrait,
    QueryOrder,
    Unset,
    Unchanged,
    ConditionalStatement,
    QuerySelect,
    JoinType,
    RelationTrait,
    DbBackend,
    Statement,
    UpdateResult,
};
use sea_orm::ActiveValue::Set;
use sea_orm::QueryFilter;
use crate::api::doc_info::ListParams;
use crate::entity::{ doc2_doc, doc_info };
use crate::entity::doc_info::Entity;
use crate::service;

fn now() -> chrono::DateTime<FixedOffset> {
    Utc::now().with_timezone(&FixedOffset::east_opt(3600 * 8).unwrap())
}

pub struct Query;

impl Query {
    pub async fn find_doc_info_by_id(
        db: &DbConn,
        id: i64
    ) -> Result<Option<doc_info::Model>, DbErr> {
        Entity::find_by_id(id).one(db).await
    }

    pub async fn find_doc_infos(db: &DbConn) -> Result<Vec<doc_info::Model>, DbErr> {
        Entity::find().all(db).await
    }

    pub async fn find_doc_infos_by_uid(
        db: &DbConn,
        uid: i64
    ) -> Result<Vec<doc_info::Model>, DbErr> {
        Entity::find().filter(doc_info::Column::Uid.eq(uid)).all(db).await
    }

    pub async fn find_doc_infos_by_name(
        db: &DbConn,
        uid: i64,
        name: &String,
        parent_id: Option<i64>
    ) -> Result<Vec<doc_info::Model>, DbErr> {
        let mut dids = Vec::<i64>::new();
        if let Some(pid) = parent_id {
            for d2d in doc2_doc::Entity
                ::find()
                .filter(doc2_doc::Column::ParentId.eq(pid))
                .all(db).await? {
                dids.push(d2d.did);
            }
        } else {
            let doc = Entity::find()
                .filter(doc_info::Column::DocName.eq(name.clone()))
                .filter(doc_info::Column::Uid.eq(uid))
                .all(db).await?;
            if doc.len() == 0 {
                return Ok(vec![]);
            }
            assert!(doc.len() > 0);
            let d2d = doc2_doc::Entity
                ::find()
                .filter(doc2_doc::Column::Did.eq(doc[0].did))
                .all(db).await?;
            assert!(d2d.len() <= 1, "Did: {}->{}", doc[0].did, d2d.len());
            if d2d.len() > 0 {
                for d2d_ in doc2_doc::Entity
                    ::find()
                    .filter(doc2_doc::Column::ParentId.eq(d2d[0].parent_id))
                    .all(db).await? {
                    dids.push(d2d_.did);
                }
            }
        }

        Entity::find()
            .filter(doc_info::Column::DocName.eq(name.clone()))
            .filter(doc_info::Column::Uid.eq(uid))
            .filter(doc_info::Column::Did.is_in(dids))
            .filter(doc_info::Column::IsDeleted.eq(false))
            .all(db).await
    }

    pub async fn all_descendent_ids(db: &DbConn, doc_ids: &Vec<i64>) -> Result<Vec<i64>, DbErr> {
        let mut dids = doc_ids.clone();
        let mut i: usize = 0;
        loop {
            if dids.len() == i {
                break;
            }

            for d in doc2_doc::Entity
                ::find()
                .filter(doc2_doc::Column::ParentId.eq(dids[i]))
                .all(db).await? {
                dids.push(d.did);
            }
            i += 1;
        }
        Ok(dids)
    }

    pub async fn find_doc_infos_by_params(
        db: &DbConn,
        params: ListParams
    ) -> Result<Vec<doc_info::Model>, DbErr> {
        // Setup paginator
        let mut sql: String =
            "
        select 
        a.did,
        a.uid,
        a.doc_name,
        a.location,
        a.size,
        a.type,
        a.created_at,
        a.updated_at,
        a.is_deleted
        from 
        doc_info as a
        ".to_owned();

        let mut cond: String = format!(" a.uid={} and a.is_deleted=False ", params.uid);

        if let Some(kb_id) = params.filter.kb_id {
            sql.push_str(
                &format!(" inner join kb2_doc on kb2_doc.did = a.did and kb2_doc.kb_id={}", kb_id)
            );
        }
        if let Some(folder_id) = params.filter.folder_id {
            sql.push_str(
                &format!(" inner join doc2_doc on a.did = doc2_doc.did and doc2_doc.parent_id={}", folder_id)
            );
        }
        // Fetch paginated posts
        if let Some(tag_id) = params.filter.tag_id {
            let tag = service::tag_info::Query
                ::find_tag_info_by_id(tag_id, &db).await
                .unwrap()
                .unwrap();
            if tag.folder_id > 0 {
                sql.push_str(
                    &format!(
                        " inner join doc2_doc on a.did = doc2_doc.did and doc2_doc.parent_id={}",
                        tag.folder_id
                    )
                );
            }
            if tag.regx.len() > 0 {
                cond.push_str(&format!(" and (type='{}' or doc_name ~ '{}') ", tag.tag_name, tag.regx));
            }
        }

        if let Some(keywords) = params.filter.keywords {
            cond.push_str(&format!(" and doc_name like '%{}%'", keywords));
        }
        if cond.len() > 0 {
            sql.push_str(&" where ");
            sql.push_str(&cond);
        }
        let mut orderby = params.sortby.clone();
        if orderby.len() == 0 {
            orderby = "updated_at desc".to_owned();
        }
        sql.push_str(&format!(" order by {}", orderby));
        let mut page_size: u32 = 30;
        if let Some(pg_sz) = params.per_page {
            page_size = pg_sz;
        }
        let mut page: u32 = 0;
        if let Some(pg) = params.page {
            page = pg;
        }
        sql.push_str(&format!(" limit {} offset {} ;", page_size, page * page_size));

        print!("{}", sql);
        Entity::find()
            .from_raw_sql(Statement::from_sql_and_values(DbBackend::Postgres, sql, vec![]))
            .all(db).await
    }

    pub async fn find_doc_infos_in_page(
        db: &DbConn,
        page: u64,
        posts_per_page: u64
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
    pub async fn mv_doc_info(db: &DbConn, dest_did: i64, dids: &[i64]) -> Result<(), DbErr> {
        for did in dids {
            let d = doc2_doc::Entity
                ::find()
                .filter(doc2_doc::Column::Did.eq(did.to_owned()))
                .all(db).await?;

            let _ = (doc2_doc::ActiveModel {
                id: Set(d[0].id),
                did: Set(did.to_owned()),
                parent_id: Set(dest_did),
            }).update(db).await?;
        }

        Ok(())
    }

    pub async fn place_doc(
        db: &DbConn,
        dest_did: i64,
        did: i64
    ) -> Result<doc2_doc::ActiveModel, DbErr> {
        (doc2_doc::ActiveModel {
            id: Default::default(),
            parent_id: Set(dest_did),
            did: Set(did),
        }).save(db).await
    }

    pub async fn create_doc_info(
        db: &DbConn,
        form_data: doc_info::Model
    ) -> Result<doc_info::ActiveModel, DbErr> {
        (doc_info::ActiveModel {
            did: Default::default(),
            uid: Set(form_data.uid.to_owned()),
            doc_name: Set(form_data.doc_name.to_owned()),
            size: Set(form_data.size.to_owned()),
            r#type: Set(form_data.r#type.to_owned()),
            location: Set(form_data.location.to_owned()),
            thumbnail_base64: Default::default(),
            created_at: Set(form_data.created_at.to_owned()),
            updated_at: Set(form_data.updated_at.to_owned()),
            is_deleted: Default::default(),
        }).save(db).await
    }

    pub async fn update_doc_info_by_id(
        db: &DbConn,
        id: i64,
        form_data: doc_info::Model
    ) -> Result<doc_info::Model, DbErr> {
        let doc_info: doc_info::ActiveModel = Entity::find_by_id(id)
            .one(db).await?
            .ok_or(DbErr::Custom("Cannot find.".to_owned()))
            .map(Into::into)?;

        (doc_info::ActiveModel {
            did: doc_info.did,
            uid: Set(form_data.uid.to_owned()),
            doc_name: Set(form_data.doc_name.to_owned()),
            size: Set(form_data.size.to_owned()),
            r#type: Set(form_data.r#type.to_owned()),
            location: Set(form_data.location.to_owned()),
            thumbnail_base64: doc_info.thumbnail_base64,
            created_at: doc_info.created_at,
            updated_at: Set(now()),
            is_deleted: Default::default(),
        }).update(db).await
    }

    pub async fn delete_doc_info(db: &DbConn, doc_ids: &Vec<i64>) -> Result<UpdateResult, DbErr> {
        let mut dids = doc_ids.clone();
        let mut i: usize = 0;
        loop {
            if dids.len() == i {
                break;
            }
            let mut doc: doc_info::ActiveModel = Entity::find_by_id(dids[i])
                .one(db).await?
                .ok_or(DbErr::Custom(format!("Can't find doc:{}", dids[i])))
                .map(Into::into)?;
            doc.updated_at = Set(now());
            doc.is_deleted = Set(true);
            let _ = doc.update(db).await?;

            for d in doc2_doc::Entity
                ::find()
                .filter(doc2_doc::Column::ParentId.eq(dids[i]))
                .all(db).await? {
                dids.push(d.did);
            }
            let _ = doc2_doc::Entity
                ::delete_many()
                .filter(doc2_doc::Column::ParentId.eq(dids[i]))
                .exec(db).await?;
            let _ = doc2_doc::Entity
                ::delete_many()
                .filter(doc2_doc::Column::Did.eq(dids[i]))
                .exec(db).await?;
            i += 1;
        }
        crate::service::kb_info::Mutation::remove_docs(&db, dids, None).await
    }

    pub async fn rename(db: &DbConn, doc_id: i64, name: &String) -> Result<doc_info::Model, DbErr> {
        let mut doc: doc_info::ActiveModel = Entity::find_by_id(doc_id)
            .one(db).await?
            .ok_or(DbErr::Custom(format!("Can't find doc:{}", doc_id)))
            .map(Into::into)?;
        doc.updated_at = Set(now());
        doc.doc_name = Set(name.clone());
        doc.update(db).await
    }

    pub async fn delete_all_doc_infos(db: &DbConn) -> Result<DeleteResult, DbErr> {
        Entity::delete_many().exec(db).await
    }
}
