use sea_orm::entity::prelude::*;
use serde::{Deserialize, Serialize};

#[derive(Clone, Debug, PartialEq, Eq, DeriveEntityModel, Deserialize, Serialize)]
#[sea_orm(table_name = "tag_info")]
pub struct Model {
    #[sea_orm(primary_key)]
    #[serde(skip_deserializing)]
    pub uid: i64,
    pub tag_name: String,
    pub regx: String,
    pub color: i64,
    pub icon: i64,
    pub dir: String,

    pub created_at: DateTimeWithTimeZone,
    pub updated_at: DateTimeWithTimeZone,
    #[sea_orm(soft_delete_column)]
    pub is_deleted: bool,
}

#[derive(Copy, Clone, Debug, EnumIter, DeriveRelation)]
pub enum Relation {}

impl Related<super::doc_info::Entity> for Entity {
    fn to() -> RelationDef {
        super::tag_2_doc::Relation::DocInfo.def()
    }

    fn via() -> Option<RelationDef> {
        Some(super::tag_2_doc::Relation::Tag.def().rev())
    }
}

impl ActiveModelBehavior for ActiveModel {}